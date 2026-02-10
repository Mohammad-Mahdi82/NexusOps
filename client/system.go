package main

import (
	"fmt"
	"golang.org/x/sys/windows/registry"
	"os"
	"os/exec"
	"path/filepath" // Use this for safer path handling
	"syscall"
	"unsafe"
)

// AppName is used for the Mutex name and the Registry key
const AppName = "NexusOpsSentry"

var (
	kernel32        = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutex = kernel32.NewProc("CreateMutexW")
)

func createMutex(name string) bool {
	lpName, _ := syscall.UTF16PtrFromString(name)
	_, _, err := procCreateMutex.Call(0, 1, uintptr(unsafe.Pointer(lpName)))
	return err == nil || err.(syscall.Errno) != 183 // 183 = Already Exists
}

func runSilentCommand(name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(name, arg...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}

func setAutoStart(serverAddr string) {
	exePath, _ := os.Executable()
	// Adding quotes around exePath is vital in case the user's name has a space
	runCmd := fmt.Sprintf("\"%s\" -server %s", exePath, serverAddr)
	key, _, _ := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	defer key.Close()
	_ = key.SetStringValue(AppName, runCmd)
}

func handleUninstall() {
	key, _ := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if key != 0 {
		key.DeleteValue(AppName)
		key.Close()
	}

	// Dynamic name detection: automatically finds "Sentry.exe"
	exePath, _ := os.Executable()
	thisExeName := filepath.Base(exePath)

	// Clean exit: Kill the process after cleaning the registry
	fmt.Printf("Stopping %s and removing from startup...\n", thisExeName)
	_ = runSilentCommand("taskkill", "/F", "/IM", thisExeName).Run()
	os.Exit(0)
}
