package notify

import (
	"fmt"
	"os/exec"
	"runtime"
)

func Send(title, body string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("notify-send"); err != nil {
			return
		}
		cmd = exec.Command("notify-send", "-a", "TermiChat", title, body)
	case "darwin":
		if _, err := exec.LookPath("osascript"); err != nil {
			return
		}
		script := fmt.Sprintf(`display notification %q with title %q`, body, title)
		cmd = exec.Command("osascript", "-e", script)
	case "windows":
		if _, err := exec.LookPath("powershell"); err != nil {
			return
		}
		script := fmt.Sprintf(`[reflection.assembly]::LoadWithPartialName('System.Windows.Forms') | Out-Null; [System.Windows.Forms.MessageBox]::Show(%q, %q) | Out-Null`, body, title)
		cmd = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	default:
		return
	}
	_ = cmd.Start()
}
