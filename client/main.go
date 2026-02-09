package main

import (
	"context"
	"flag"
	"fmt"
	"log"
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
	user32                       = syscall.NewLazyDLL("user32.dll")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
)

const appName = "NexusOpsSentry"

// getActiveProcessName detects the EXE of the window currently in focus
func getActiveProcessName() string {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return "Idle"
	}

	var processID uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&processID)))

	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", processID), "/NH", "/FO", "CSV")
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

	// Filter common Windows noise, Terminals, and System tools
	idleProcesses := map[string]bool{
		"explorer.exe":            true,
		"cmd.exe":                 true,
		"powershell.exe":          true,
		"pwsh.exe":                true,
		"windowsterminal.exe":     true,
		"conhost.exe":             true,
		"taskhostw.exe":           true,
		"lockapp.exe":             true,
		"shellexperiencehost.exe": true,
		"taskmgr.exe":             true,
		"":                        true,
	}

	if idleProcesses[lowName] {
		return "Idle"
	}

	return name
}

func setAutoStart() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	_ = key.SetStringValue(appName, exePath)
}

func removeAutoStart() {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		log.Printf("Failed to open registry for uninstall: %v", err)
		return
	}
	defer key.Close()
	err = key.DeleteValue(appName)
	if err != nil {
		log.Printf("Registry key not found or already removed: %v", err)
	} else {
		log.Println("Successfully removed from startup.")
	}
}

func main() {
	serverAddr := flag.String("server", "localhost:50051", "Server IP:Port")
	uninstall := flag.Bool("uninstall", false, "Remove from startup and exit")
	flag.Parse()

	if *uninstall {
		removeAutoStart()
		return
	}

	setAutoStart()
	pcName, _ := os.Hostname()

	// --- INFINITE RECONNECT LOOP ---
	for {
		log.Printf("Connecting to server at %s...", *serverAddr)
		conn, err := grpc.Dial(*serverAddr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		)

		if err != nil {
			log.Printf("Server not reachable. Retrying in 10s...")
			time.Sleep(10 * time.Second)
			continue
		}

		client := pb.NewNexusServiceClient(conn)
		err = runSentryStream(client, pcName)

		log.Printf("Stream closed/error: %v. Reconnecting in 5s...", err)
		conn.Close()
		time.Sleep(5 * time.Second)
	}
}

func runSentryStream(client pb.NexusServiceClient, pcName string) error {
	stream, err := client.StreamSession(context.Background())
	if err != nil {
		return err
	}
	log.Printf("Stream active for PC: %s", pcName)

	for {
		currentGame := getActiveProcessName()

		// 1. Send Heartbeat
		err := stream.Send(&pb.Heartbeat{
			PcId:        pcName,
			CurrentGame: currentGame,
			Timestamp:   time.Now().Unix(),
		})
		if err != nil {
			return err
		}

		// 2. Receive command from Server
		resp, err := stream.Recv()
		if err == nil && resp.CloseActiveGame && currentGame != "Idle" {
			log.Printf("SERVER COMMAND: Closing active process [%s]", currentGame)
			// Force kill the game/process EXE
			exec.Command("taskkill", "/F", "/IM", currentGame).Run()
		}

		time.Sleep(5 * time.Second)
	}
}
