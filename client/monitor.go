package main

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

var (
	user32                       = syscall.NewLazyDLL("user32.dll")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
)

// GetActiveProcessName retrieves the name of the executable currently in focus.
func GetActiveProcessName() string {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return "Idle"
	}

	var processID uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&processID)))

	// NH = No Header, FO CSV = Comma Separated for easy parsing
	cmd := runSilentCommand("tasklist", "/FI", fmt.Sprintf("PID eq %d", processID), "/NH", "/FO", "CSV")
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		return "Idle"
	}

	// tasklist CSV output looks like: "sentry.exe","1234","Console","1","5,000 K"
	fields := strings.Split(string(output), ",")
	if len(fields) == 0 {
		return "Idle"
	}

	name := strings.Trim(fields[0], "\"")

	if isSystemProcess(strings.ToLower(name)) {
		return "Idle"
	}
	return name
}

// isSystemProcess filters out background Windows tasks so they don't count as "Games".
func isSystemProcess(name string) bool {
	// Use a map for O(1) lookup speed instead of looping through a slice every 5 seconds
	systemProcesses := map[string]bool{
		"":                        true,
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
		"steam.exe":               true, // Launcher itself is idle
		"steamwebhelper.exe":      true,
		"epicgameslauncher.exe":   true,
		"origin.exe":              true,
		"battle.net.exe":          true,
		"discord.exe":             true,
	}

	return systemProcesses[name]
}
