// ApkTool changes the package name and the label of an APK.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"apktool/internal/apk"
)

const (
	cReset  = "\x1b[0m"
	cBold   = "\x1b[1m"
	cDim    = "\x1b[2m"
	cGreen  = "\x1b[32m"
	cCyan   = "\x1b[36m"
	cYellow = "\x1b[33m"
	cRed    = "\x1b[31m"
)

func main() {
	setupConsole()
	if err := run(); err != nil {
		fmt.Println()
		fmt.Println(cRed + cBold + "  ERRORE: " + cReset + err.Error())
	}
	fmt.Println()
	fmt.Print(cDim + "  Premi Invio per chiudere..." + cReset)
	readLine()
}

func banner() {
	fmt.Println()
	fmt.Println(cCyan + cBold + "   ╔══════════════════════════════════════════╗")
	fmt.Println("   ║                 ApkTool                  ║")
	fmt.Println("   ║   Cambia package name ed etichetta APK   ║")
	fmt.Println("   ╚══════════════════════════════════════════╝" + cReset)
	fmt.Println()
}

func run() error {
	banner()

	// 1. APK selection
	fmt.Println(cBold + "  1) Seleziona l'APK" + cReset)
	if pickHint != "" {
		fmt.Println(cDim + "     " + pickHint + cReset)
	}
	path := pickApkFile()
	var info apk.Info
	for {
		if path == "" {
			fmt.Print("     Percorso dell'APK (puoi anche trascinarlo qui): ")
			path = cleanPath(readLine())
		}
		if !isApkFile(path) {
			fmt.Println(cRed + "     File non valido o inesistente." + cReset)
			path = ""
			continue
		}
		var err error
		if info, err = apk.Read(path); err == nil {
			break
		}
		fmt.Println(cRed + "     Impossibile leggere l'APK: " + err.Error() + cReset)
		path = ""
	}
	fmt.Println("     " + cGreen + "✔ " + cReset + path)
	fmt.Println()

	// 2. package name
	fmt.Println(cBold + "  2) Package name" + cReset)
	fmt.Println("     Attuale: " + cYellow + info.Package + cReset)
	newPkg := info.Package
	for {
		fmt.Print("     Nuovo package name " + cDim + "(Invio = lascia invariato)" + cReset + ": ")
		s := strings.TrimSpace(readLine())
		if s == "" {
			break
		}
		if validPackage(s) {
			newPkg = s
			break
		}
		fmt.Println(cRed + "     Formato non valido. Esempio: it.azienda.app.test (lettere, numeri e _, almeno un punto)." + cReset)
	}
	fmt.Println()

	// 3. label
	fmt.Println(cBold + "  3) Etichetta (nome visibile dell'app)" + cReset)
	fmt.Println("     Attuale: " + cYellow + displayLabel(info.Label) + cReset)
	fmt.Print("     Nuova etichetta " + cDim + "(Invio = lascia invariata)" + cReset + ": ")
	newLabel := strings.TrimSpace(readLine())
	if newLabel == "" {
		newLabel = info.Label
	}
	fmt.Println()

	// 4. destination
	def := downloadsDir()
	fmt.Println(cBold + "  4) Cartella di destinazione" + cReset)
	fmt.Println("     Predefinita: " + cYellow + def + cReset)
	var dest string
	for {
		fmt.Print("     Percorso " + cDim + "(Invio = usa la predefinita)" + cReset + ": ")
		dest = cleanPath(readLine())
		if dest == "" {
			dest = def
		}
		if err := os.MkdirAll(dest, 0o755); err == nil {
			break
		}
		fmt.Println(cRed + "     Impossibile usare questa cartella." + cReset)
	}
	outPath := uniquePath(filepath.Join(dest, fileNameFor(newLabel, time.Now())))
	fmt.Println()

	fmt.Println(cBold + "  Riepilogo" + cReset)
	fmt.Printf("     Package:   %s  →  %s%s%s\n", info.Package, cGreen, newPkg, cReset)
	fmt.Printf("     Etichetta: %s  →  %s%s%s\n", displayLabel(info.Label), cGreen, displayLabel(newLabel), cReset)
	fmt.Printf("     File:      %s\n", outPath)
	fmt.Println()

	// 5. processing
	fmt.Println(cDim + "  Creazione e firma dell'APK..." + cReset)
	if err := apk.Rename(path, outPath, newPkg, newLabel); err != nil {
		return fmt.Errorf("creazione dell'APK non riuscita: %w", err)
	}
	fmt.Println(cGreen + cBold + "  ✔ APK creato:" + cReset)
	fmt.Println("     " + outPath)
	fmt.Println()
	fmt.Println(cDim + "     Nota: l'APK è firmato con una chiave di test, quindi si installa" + cReset)
	fmt.Println(cDim + "     come app separata e non come aggiornamento dell'originale." + cReset)
	revealInExplorer(outPath)
	return nil
}

func displayLabel(label string) string {
	if label == "" {
		return "(nessuna)"
	}
	return label
}
