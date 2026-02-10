package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"

	pb "github.com/Mohammad-Mahdi82/NexusOps/pkg/monitor"
	"golang.org/x/sys/windows/registry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutex              = kernel32.NewProc("CreateMutexW")
	user32                       = syscall.NewLazyDLL("user32.dll")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
)

const appName = "NexusOpsSentry"

// createMutex prevents multiple instances of the sentry from running simultaneously.
func createMutex(name string) (uintptr, error) {
	lpName, _ := syscall.UTF16PtrFromString(name)
	handle, _, err := procCreateMutex.Call(0, 1, uintptr(unsafe.Pointer(lpName)))

	if err != nil && err.(syscall.Errno) == 183 {
		return 0, fmt.Errorf("mutex already exists")
	}

	return handle, nil
}

func runSilentCommand(name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(name, arg...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}

func getActiveProcessName() string {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return "Idle"
	}

	var processID uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&processID)))

	cmd := runSilentCommand("tasklist", "/FI", fmt.Sprintf("PID eq %d", processID), "/NH", "/FO", "CSV")
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		return "Idle"
	}

	fields := strings.Split(string(output), ",")
	if len(fields) < 1 {
		return "Idle"
	}

	name := strings.Trim(fields[0], "\"")
	lowName := strings.ToLower(name)

	idleProcesses := map[string]bool{
		"explorer.exe":            true,
		"searchhost.exe":          true,
		"shellexperiencehost.exe": true,
		"lockapp.exe":             true,
		"taskhostw.exe":           true,
		"cmd.exe":                 true,
		"powershell.exe":          true,
		"pwsh.exe":                true,
		"windowsterminal.exe":     true,
		"conhost.exe":             true,
		"taskmgr.exe":             true,
		"steam.exe":               true,
		"steamwebhelper.exe":      true,
		"epicgameslauncher.exe":   true,
		"origin.exe":              true,
		"battle.net.exe":          true,
		"":                        true,
	}

	if idleProcesses[lowName] {
		return "Idle"
	}

	return name
}

func setAutoStart(serverAddr string) {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	runCmd := fmt.Sprintf("\"%s\" -server %s", exePath, serverAddr)

	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	_ = key.SetStringValue(appName, runCmd)
}

func removeAutoStart() {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	_ = key.DeleteValue(appName)
}

func main() {
	// 1. Parse flags FIRST so -uninstall can run regardless of Mutex
	serverAddr := flag.String("server", "localhost:50051", "Server IP:Port")
	uninstall := flag.Bool("uninstall", false, "Remove from startup and stop running process")
	flag.Parse()

	if *uninstall {
		fmt.Println("Uninstalling Nexus Sentry...")
		removeAutoStart()

		exePath, _ := os.Executable()
		parts := strings.Split(exePath, "\\")
		thisExeName := parts[len(parts)-1]

		// Kill other instances
		_ = runSilentCommand("taskkill", "/F", "/IM", thisExeName).Run()

		fmt.Println("Registry cleared and processes stopped.")
		os.Exit(0)
	}

	// 2. NOW check for Singleton status for the monitoring instance
	_, err := createMutex("Global\\NexusOpsSentryMutex")
	if err != nil {
		// Silent exit if already running and not uninstalling
		os.Exit(0)
	}

	setAutoStart(*serverAddr)
	pcName, _ := os.Hostname()

	for {
		conn, err := grpc.Dial(*serverAddr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		)

		if err != nil {
			time.Sleep(10 * time.Second)
			continue
		}

		client := pb.NewNexusServiceClient(conn)
		_ = runSentryStream(client, pcName)

		conn.Close()
		time.Sleep(5 * time.Second)
	}
}

func runSentryStream(client pb.NexusServiceClient, pcName string) error {
	stream, err := client.StreamSession(context.Background())
	if err != nil {
		return err
	}

	for {
		currentGame := getActiveProcessName()

		err := stream.Send(&pb.Heartbeat{
			PcId:        pcName,
			CurrentGame: currentGame,
			Timestamp:   time.Now().Unix(),
		})
		if err != nil {
			return err
		}

		resp, err := stream.Recv()
		if err == nil && resp.CloseActiveGame && currentGame != "Idle" {
			_ = runSilentCommand("taskkill", "/F", "/IM", currentGame).Run()
			currentGame = "Idle"
		}

		time.Sleep(5 * time.Second)
	}
}
