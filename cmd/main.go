package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"unsafe"

	"github.com/getlantern/systray"
	"golang.org/x/sys/windows/registry"
)

const (
	defaultPort           = 9876
	defaultShutdownTimout = 10
	runKeyPath            = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValueName          = "PCAgent"
)

type Config struct {
	Port               int  `json:"port"`
	ShutdownTimeoutSec int  `json:"shutdown_timeout_sec"`
	Autostart          bool `json:"autostart"`
}

type App struct {
	cfgMu      sync.RWMutex
	cfg        Config
	server     *Server
	mStatus    *systray.MenuItem
	mAutostart *systray.MenuItem
}

type Server struct {
	mu   sync.Mutex
	srv  *http.Server
	port int
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Println("config load error:", err)
	}

	app := &App{
		cfg:    cfg,
		server: &Server{},
	}

	if err := app.server.Start(cfg.Port, app); err != nil {
		fmt.Println("server start error:", err)
	}

	// Apply autostart setting
	if err := setAutostart(cfg.Autostart); err != nil {
		fmt.Println("autostart error:", err)
	}

	systray.Run(func() { onReady(app) }, func() { onExit(app) })
}

func onReady(app *App) {
	systray.SetIcon(generateTrayIcon())
	systray.SetTitle("PC Agent")
	systray.SetTooltip("PC Agent - Remote Shutdown Service")

	cfg := app.getConfig()

	// Status line (disabled, just shows info)
	app.mStatus = systray.AddMenuItem(fmt.Sprintf("Port: %d | Timeout: %ds", cfg.Port, cfg.ShutdownTimeoutSec), "Current settings")
	app.mStatus.Disable()

	systray.AddSeparator()

	// Config management
	mOpenConfig := systray.AddMenuItem("Edit Config", "Open config.json in notepad")
	mReload := systray.AddMenuItem("Reload Config", "Reload settings from config.json")
	mOpenFolder := systray.AddMenuItem("Open App Folder", "Open folder with exe and config")

	systray.AddSeparator()

	// Autostart toggle
	app.mAutostart = systray.AddMenuItem(getAutostartText(cfg.Autostart), "Toggle Windows autostart")

	systray.AddSeparator()

	// About & Exit
	mAbout := systray.AddMenuItem("About", "About PC Agent")
	mQuit := systray.AddMenuItem("Exit", "Exit application")

	go func() {
		for {
			select {
			case <-mOpenConfig.ClickedCh:
				if path, err := configPath(); err == nil {
					// Create config if not exists
					if _, err := os.Stat(path); os.IsNotExist(err) {
						saveConfig(app.getConfig())
					}
					exec.Command("notepad", path).Start()
				}
			case <-mReload.ClickedCh:
				if newCfg, err := loadConfig(); err == nil {
					oldPort := app.getConfig().Port
					app.setConfig(newCfg)

					// Restart server if port changed
					if oldPort != newCfg.Port {
						if err := app.server.Start(newCfg.Port, app); err != nil {
							fmt.Println("Failed to restart server:", err)
						}
					}

					// Apply autostart
					setAutostart(newCfg.Autostart)

					// Update UI
					app.mStatus.SetTitle(fmt.Sprintf("Port: %d | Timeout: %ds", newCfg.Port, newCfg.ShutdownTimeoutSec))
					app.mAutostart.SetTitle(getAutostartText(newCfg.Autostart))
				}
			case <-mOpenFolder.ClickedCh:
				if path, err := configPath(); err == nil {
					dir := filepath.Dir(path)
					exec.Command("explorer", dir).Start()
				}
			case <-app.mAutostart.ClickedCh:
				cfg := app.getConfig()
				newAutostart := !cfg.Autostart
				if err := setAutostart(newAutostart); err == nil {
					cfg.Autostart = newAutostart
					app.setConfig(cfg)
					saveConfig(cfg)
					app.mAutostart.SetTitle(getAutostartText(newAutostart))
				}
			case <-mAbout.ClickedCh:
				cfg := app.getConfig()
				showAbout(cfg)
			case <-mQuit.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

func getInfoText(cfg Config) string {
	return fmt.Sprintf(`PC Agent v1.0
Remote Shutdown Service

Current Settings:
• Port: %d
• Shutdown Timeout: %d seconds
• Autostart: %v

API Endpoints:
• GET  /ping     - Health check
• POST /shutdown - Schedule shutdown

Example:
curl http://localhost:%d/shutdown`,
		cfg.Port, cfg.ShutdownTimeoutSec, cfg.Autostart, cfg.Port)
}

func showAbout(cfg Config) {
	showMessageBox("About PC Agent", getInfoText(cfg))
}

func showMessageBox(title, text string) {
	const (
		MB_OK              = 0x00000000
		MB_ICONINFORMATION = 0x00000040
	)

	user32 := syscall.NewLazyDLL("user32.dll")
	messageBox := user32.NewProc("MessageBoxW")

	titlePtr, _ := syscall.UTF16PtrFromString(title)
	textPtr, _ := syscall.UTF16PtrFromString(text)

	messageBox.Call(
		0, // hWnd - no parent window
		uintptr(unsafe.Pointer(textPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(MB_OK|MB_ICONINFORMATION),
	)
}

func getAutostartText(enabled bool) string {
	if enabled {
		return "Autostart ✓"
	}
	return "Autostart"
}

func onExit(app *App) {
	app.server.Stop()
}

func (a *App) getConfig() Config {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return a.cfg
}

func (a *App) setConfig(cfg Config) {
	a.cfgMu.Lock()
	a.cfg = cfg
	a.cfgMu.Unlock()
}

func (s *Server) Start(port int, app *App) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.srv != nil && s.port == port {
		return nil
	}

	if s.srv != nil {
		_ = s.srv.Close()
		s.srv = nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		cfg := app.getConfig()
		if err := scheduleShutdown(cfg.ShutdownTimeoutSec); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("shutdown scheduled"))
	})

	addr := fmt.Sprintf(":%d", port)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	s.srv = srv
	s.port = port

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Println("listen error:", err)
		}
	}()

	return nil
}

func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv != nil {
		_ = s.srv.Close()
		s.srv = nil
	}
}

func scheduleShutdown(timeoutSec int) error {
	if timeoutSec < 0 {
		timeoutSec = 0
	}
	cmd := exec.Command("shutdown", "/s", "/t", strconv.Itoa(timeoutSec))
	return cmd.Start()
}

func configPath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	exeDir := filepath.Dir(exePath)
	return filepath.Join(exeDir, "config.json"), nil
}

func loadConfig() (Config, error) {
	cfg := Config{
		Port:               defaultPort,
		ShutdownTimeoutSec: defaultShutdownTimout,
		Autostart:          false,
	}
	path, err := configPath()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg.Autostart = getAutostartEnabled()
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func saveConfig(cfg Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func setAutostart(enabled bool) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()

	if enabled {
		exePath, err := os.Executable()
		if err != nil {
			return err
		}
		value := fmt.Sprintf("\"%s\"", exePath)
		return key.SetStringValue(runValueName, value)
	}

	if err := key.DeleteValue(runValueName); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}

func getAutostartEnabled() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	_, _, err = key.GetStringValue(runValueName)
	return err == nil
}
