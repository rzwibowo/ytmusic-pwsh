package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"time"
)

func (p *player) vlcURL(command string) string {
	base := fmt.Sprintf("http://127.0.0.1:%d/requests/status.json", p.cfg.HTTPPort)
	if command != "" {
		return base + "?command=" + command
	}
	return base
}

func (p *player) vlcRequest(command string, target any) error {
	req, err := http.NewRequest(http.MethodGet, p.vlcURL(command), nil)
	if err != nil {
		return err
	}
	token := base64.StdEncoding.EncodeToString([]byte(":" + p.cfg.HTTPPassword))
	req.Header.Set("Authorization", "Basic "+token)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("VLC HTTP status: %s", resp.Status)
	}
	if target != nil {
		return json.NewDecoder(resp.Body).Decode(target)
	}
	return nil
}

func (p *player) getVLCStatus() (*vlcStatus, error) {
	var status vlcStatus
	if err := p.vlcRequest("", &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func (p *player) startVLC() error {
	if _, err := p.getVLCStatus(); err == nil {
		return nil
	}
	exe, err := resolveExecutable(p.cfg.VLCPath)
	if err != nil {
		return fmt.Errorf("VLC not found (%s)", p.cfg.VLCPath)
	}
	fmt.Println("\nStarting VLC headless...")
	cmd := exec.Command(exe,
		"--intf", "dummy",
		"--extraintf", "http",
		"--http-port", strconv.Itoa(p.cfg.HTTPPort),
		"--http-password", p.cfg.HTTPPassword,
		"--no-video",
		"--audio-visual=none",
	)
	hideProcessWindow(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	p.vlcProcess = cmd
	if err := attachProcessToJob(cmd.Process); err != nil {
		fmt.Println("Warning: VLC could not be attached to terminal cleanup:", err)
	}
	stopSpinner := startSpinner("Waiting for VLC")
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := p.getVLCStatus(); err == nil {
			stopSpinner()
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	stopSpinner()
	p.stopVLC()
	return fmt.Errorf("VLC did not start on HTTP port %d", p.cfg.HTTPPort)
}

func (p *player) stopVLC() {
	p.autoAdvanceArmed = false
	_ = p.vlcRequest("pl_exit", nil)
	if p.vlcProcess != nil && p.vlcProcess.Process != nil {
		done := make(chan struct{})
		go func() {
			_ = p.vlcProcess.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = p.vlcProcess.Process.Kill()
		}
	}
}

func (p *player) playStream(stream string) error {
	stopSpinner := startSpinner("Opening in VLC")
	defer stopSpinner()
	if err := p.vlcRequest("pl_empty", nil); err != nil {
		return err
	}
	return p.vlcRequest("in_play&input="+url.QueryEscape(stream), nil)
}
