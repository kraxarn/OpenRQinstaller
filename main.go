package main

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"fyne.io/fyne"
	"fyne.io/fyne/app"
	"fyne.io/fyne/dialog"
	"fyne.io/fyne/layout"
	"fyne.io/fyne/widget"
)

const appName = "OpenRQ"

// Gets the username of the current user
// TODO: Cache this as username probably doesn't change during execution
func GetUsername() string {
	currentUser, err := user.Current()
	if err != nil {
		panic(err)
	}
	// Windows puts the pc name in the username for some reason
	if strings.Contains(currentUser.Username, "\\") {
		index := strings.LastIndex(currentUser.Username, "\\") + 1
		currentUser.Username = currentUser.Username[index:]
	}
	return currentUser.Username
}

func GetTempPath() string {
	// TODO: ioutil.TempDir
	// darwin doesn't need username (probably)
	if runtime.GOOS == "darwin" {
		return "/tmp/"
	}
	// Try to match platform
	var dir string
	if runtime.GOOS == "windows" {
		dir = "C:/Users/%s/AppData/Local/Temp/"
	} else {
		dir = "/home/%s/.cache/"
	}
	// Get full temp path
	return fmt.Sprintf(dir, GetUsername())
}

// Gets the install path
func GetInstallPath() string {
	// Default current directory
	dir := "%s/%s/"
	// Try to match platform
	switch runtime.GOOS {
	case "windows":
		dir = "C:/Users/%s/AppData/Local/%s/"
	case "linux":
		dir = "/home/%s/.local/share/%s/"
	case "darwin":
		return fmt.Sprintf("/Applications/%s/", appName)
	}
	// Return formatted string
	return fmt.Sprintf(dir, GetUsername(), appName)
}

func GetFileFromPath(path string) string {
	// Try to get last index of /
	lastIndex := strings.LastIndex(path, "/") + 1
	// -1 + 1 = 0, so lastIndex is 0 if failed
	if lastIndex == 0 {
		return path
	}
	// Return final string
	return path[lastIndex:]
}

// Attempts to extract input zip file to output destination
func Extract(input []byte, output string, progress *widget.ProgressBar) error {
	// Try to open file
	reader, err := zip.NewReader(bytes.NewReader(input), int64(len(input)))
	if err != nil {
		return err
	}
	// Helper function to extract each file in a zip
	extractAndWrite := func(file *zip.File) error {
		// Open file for reading
		readCloser, err := file.Open()
		if err != nil {
			return err
		}
		// Close file when we're done
		defer func() {
			if err := readCloser.Close(); err != nil {
				panic(err)
			}
		}()
		// Get full output path
		path := filepath.Join(output, file.Name)
		// If it's just a directory, create it only
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(path, file.Mode()); err != nil {
				return err
			}
			// If it's a file, actually extract it
		} else {
			// Create directory for file if needed
			if err := os.MkdirAll(filepath.Dir(path), file.Mode()); err != nil {
				return err
			}
			// Create output file
			outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
			if err != nil {
				return err
			}
			// Close output file after we're done
			defer func() {
				if err := outFile.Close(); err != nil {
					panic(err)
				}
			}()
			// Copy to output file
			_, err = io.Copy(outFile, readCloser)
			if err != nil {
				return err
			}
		}
		// Nothing went wrong, no error
		return nil
	}
	// Loop through all files in zip
	for i := 0; i < len(reader.File); i++ {
		// Get current file
		file := reader.File[i]
		// Update progress
		progress.SetValue(float64(i+1) / float64(len(reader.File)))
		// Attempt to extract file
		err := extractAndWrite(file)
		if err != nil {
			return err
		}
	}

	return nil
}

// Get the name of the executable, often just the app name
func GetExecutableName() string {
	execName := appName
	// Add exe to executable if windows
	if runtime.GOOS == "windows" {
		execName += ".exe"
	}
	return execName
}

func Copy(input, output string) error {
	// Open file to copy from
	inFile, err := os.Open(input)
	if err != nil {
		return err
	}
	// Close file when we're done
	defer func() {
		if err := inFile.Close(); err != nil {
			panic(err)
		}
	}()

	// Remove output if it already exists
	// (if there's no error, it probably exists)
	if _, err := os.Stat(output); err == nil {
		return nil
	}

	// Create new file we're copying to
	outFile, err := os.Create(output)
	if err != nil {
		return err
	}
	// Close file when we're done
	defer func() {
		if err := outFile.Close(); err != nil {
			panic(err)
		}
	}()

	// Copy content to new file
	if _, err := io.Copy(outFile, inFile); err != nil {
		return err
	}
	// Everything went fine
	return nil
}

func Install(progress *widget.ProgressBar, status *widget.Label) error {
	// Create install directory if needed
	if err := os.MkdirAll(GetInstallPath(), 0700); err != nil {
		return err
	}

	data, err := base64.StdEncoding.DecodeString(appData)
	if err != nil {
		return err
	}

	status.SetText(fmt.Sprintf("Installing..."))
	return Extract(data, GetInstallPath(), progress)
}

func GetShortcutLocation() string {
	switch runtime.GOOS {
	case "linux":
		return fmt.Sprintf("/home/%s/.local/share/applications/%s.desktop",
			GetUsername(), strings.ToLower(appName))
	case "windows":
		return fmt.Sprintf("C:/Users/%s/AppData/Roaming/Microsoft/Windows/Start Menu/Programs/%s.lnk",
			GetUsername(), appName)
	}
	// Return empty string by default
	return ""
}

func CreateShortcut() error {
	// darwin doesn't use shortcuts
	if runtime.GOOS == "darwin" {
		return nil
	}
	// linux uses a simple desktop file
	if runtime.GOOS == "linux" {
		// Create initial shortcut text
		// (this icon doesn't contain os)
		content := fmt.Sprintf("[Desktop Entry]\nName=%s\nType=Application\nTerminal=false\nExec=%s\nIcon=%s",
			appName, GetInstallPath()+appName,
			fmt.Sprintf("%s%s", GetInstallPath(), appName))
		// Try to write to file
		if err := ioutil.WriteFile(GetShortcutLocation(), []byte(content), 0700); err != nil {
			return err
		}
		// windows uses annoying binary lnk files
	} else if runtime.GOOS == "windows" {
		// We need to create a temporary Visual Basic file and then execute it
		target := GetInstallPath() + GetExecutableName()
		icon := GetInstallPath() + appName
		vbs := fmt.Sprintf("Set link = WScript.CreateObject(\"WScript.Shell\").CreateShortcut(\"%s\")\n"+
			"link.TargetPath = \"%s\"\nlink.IconLocation = \"%s\"\nlink.Description = \"%s\"\nlink.Save",
			GetShortcutLocation(), target, icon, appName)
		// Write vbs to file
		scriptFile := GetTempPath() + "CreateShortcut.vbs"
		if err := ioutil.WriteFile(scriptFile, []byte(vbs), 0777); err != nil {
			return err
		}
		// Execute script
		cmd := exec.Command("wscript", scriptFile)
		if err := cmd.Run(); err != nil {
			return err
		}
		// Remove it
		if err := os.Remove(scriptFile); err != nil {
			return err
		}
	}

	return nil
}

// Remove application folder and shortcut
func Uninstall(status *widget.Label) error {
	// Remove application folder
	status.SetText("Uninstalling application...")
	if err := os.RemoveAll(GetInstallPath()); err != nil {
		return err
	}
	// Remove shortcut (if needed)
	status.SetText("Removing shortcut...")
	shortcut := GetShortcutLocation()
	if len(shortcut) > 0 {
		if err := os.Remove(shortcut); err != nil {
			return err
		}
	}
	return nil
}

// Return row with (un)install options, install is always last item
func GetButtonContainer(installTapped func(), uninstallTapped func()) *fyne.Container {
	// Check if directory to install to already exists
	appInstalled := false
	if _, err := os.Stat(GetInstallPath()); err == nil {
		appInstalled = true
	}
	// Helper function to toggle button enable/disable
	var toggleButtons = func(buttons []*widget.Button) {
		for _, button := range buttons {
			if button.Disabled() {
				button.Enable()
			} else {
				button.Disable()
			}
		}
	}
	// If not installed, just return an install button
	if !appInstalled {
		var button *widget.Button
		button = widget.NewButton("Install", func() {
			go func() {
				// Disable button
				button.Disable()
				// Run the main function
				installTapped()
				// Enable button again
				button.Enable()
			}()
		})
		return fyne.NewContainerWithLayout(layout.NewGridLayout(1), button)
	}
	// App is not installed, return uninstall and update buttons
	var buttons []*widget.Button
	buttons = []*widget.Button{
		widget.NewButton("Uninstall", func() {
			go func() {
				// Disable buttons
				toggleButtons(buttons)
				// Run the main function
				uninstallTapped()
				// Enable button again
				toggleButtons(buttons)
			}()
		}),
		widget.NewButton("Update", func() {
			go func() {
				// Disable buttons
				toggleButtons(buttons)
				// Run the main function
				installTapped()
				// Enable button again
				toggleButtons(buttons)
			}()
		}),
	}
	return fyne.NewContainerWithLayout(layout.NewGridLayout(2), buttons[0], buttons[1])
}

func GetLayout(parent fyne.Window) fyne.CanvasObject {
	// Install progress
	progress := widget.NewProgressBar()
	// Status message
	status := widget.NewLabel("Waiting...")

	// Main layout
	return widget.NewVBox(
		// Label with what to install
		widget.NewGroup(fmt.Sprintf("Welcome to the %s installer!", appName), status),
		// Install progress
		progress,
		// Install button
		layout.NewSpacer(),
		//btnInstall,
		GetButtonContainer(func() {
			// Install/Update
			progress.SetValue(0)
			// Attempt download
			if err := Install(progress, status); err != nil {
				dialog.ShowError(err, parent)
				status.SetText("Install failed")
				// Attempt to create shortcut
			} else if err := CreateShortcut(); err != nil {
				dialog.ShowError(err, parent)
				status.SetText("Shortcut creation failed")
			} else {
				progress.SetValue(1)
				status.SetText("Installation successful!")
			}
		}, func() {
			// Uninstall
			progress.SetValue(0)
			if err := Uninstall(status); err != nil {
				dialog.ShowError(err, parent)
				status.SetText("Uninstall failed")
			} else {
				progress.SetValue(1)
				status.SetText("Uninstall successful")
			}
		}),
	)
}

func LoadIcon() fyne.Resource {
	return fyne.NewStaticResource("icon.png", icon)
}

func main() {
	// License window to refer to later
	var licenseWindow fyne.Window

	// Create new main app
	mainApp := app.New()
	// Create window
	window := mainApp.NewWindow("Installer")
	window.Resize(fyne.Size{Width: 400, Height: 200})
	window.CenterOnScreen()
	window.SetIcon(LoadIcon())

	// Set window menu
	window.SetMainMenu(fyne.NewMainMenu(
		fyne.NewMenu("File",
			fyne.NewMenuItem("About", func() {
				dialog.ShowInformation(
					"About",
					fmt.Sprintf("OpenRQinstaller based of goInstaller v1.1\nhttps://github.com/kraxarn/goInstaller\nLicensed under BSD-3"), window)
			}),
			fyne.NewMenuItem("Licenses", func() {
				// Check if we already have a license window open
				if licenseWindow != nil {
					return
				}
				// Create window with content and reset on close
				licenseWindow = fyne.CurrentApp().NewWindow("Licenses")
				licenseWindow.Resize(fyne.Size{Width: 600, Height: 800})
				licenseWindow.CenterOnScreen()
				licenseWindow.SetPadded(true)
				licenseWindow.SetContent(widget.NewScrollContainer(widget.NewLabel(licenses)))
				licenseWindow.Show()
				licenseWindow.SetOnClosed(func() {
					licenseWindow = nil
				})
			}),
		),
	))

	// Set what to show in the window
	window.SetContent(GetLayout(window))
	// Show window
	window.ShowAndRun()
}
