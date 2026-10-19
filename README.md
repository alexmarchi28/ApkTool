# ApkTool

Programma da riga di comando per cambiare **package name** ed **etichetta** (nome visibile) di un APK Android.
È un singolo eseguibile di circa 4 MB, senza dipendenze, per Windows, Linux e macOS.
Le versioni pronte sono nella pagina **Releases** del repository.

## Uso

1. Avvia il programma. Su Windows, `ApkTool.exe` apre un terminale e subito dopo Esplora file per
   scegliere l'APK; se chiudi la finestra puoi incollare o trascinare il percorso nel terminale.
   Su Linux e macOS avvia `./apktool` da un terminale e incolla o trascina il percorso dell'APK.
2. Viene mostrato il **package name attuale**: inserisci quello nuovo (Invio = invariato).
3. Viene mostrata l'**etichetta attuale**: inserisci quella nuova (Invio = invariata).
4. Scegli la cartella di destinazione (Invio = cartella **Download**, indicata come predefinita).
5. L'APK viene creato, firmato e verificato; su Windows alla fine Esplora file si apre sul file creato.

Il file generato si chiama `{etichetta}_{AAAAMMGG}.apk`, con l'etichetta in minuscolo e
underscore al posto di spazi e punteggiatura (es. `la_mia_app_test_20261019.apk`).
Se il file esiste già viene aggiunto `_2`, `_3`, ...

### Cosa viene modificato

Solo `AndroidManifest.xml`, direttamente nel suo formato binario; tutti gli altri file
dell'APK (codice, risorse, librerie) vengono copiati byte per byte.

- `package` del manifest. Come fa `aapt --rename-manifest-package`, i nomi di classe relativi
  (`.MainActivity`) diventano assoluti con il vecchio package, quindi l'app continua a funzionare.
- `android:authorities` dei provider e i permessi che iniziano con il vecchio package
  (altrimenti l'APK non si installa accanto all'originale).
- `android:label` dell'applicazione e dell'activity di avvio (se ne ha una propria).
  La nuova etichetta è uguale in tutte le lingue; se l'etichetta resta invariata,
  le traduzioni originali restano.

Poi l'APK viene allineato (come `zipalign`), firmato con gli schemi v1 e v2 e riletto per
controllare firma, package ed etichetta prima di essere salvato.

### Limiti

- L'APK viene firmato con una **chiave di debug** fissa, incorporata nel programma: si installa
  come app separata e non può aggiornare l'app originale, ma le copie create con ApkTool si
  aggiornano tra loro.
- I servizi legati al package o alla firma originale (notifiche push, login con account esterni,
  chiavi API) di solito non funzionano nella copia.
- Non gestisce AAB o split APK. L'APK viene elaborato in memoria.
- Gli eseguibili non hanno firma digitale. Su Windows, se l'exe arriva da un download, SmartScreen
  può mostrare un avviso ("Ulteriori informazioni" → "Esegui comunque"); vedi [SMARTSCREEN.md](SMARTSCREEN.md).
  Su macOS il file scaricato si sblocca con `xattr -d com.apple.quarantine apktool`.

## Compilazione

Requisiti: [Go](https://go.dev/dl/) 1.24 o successivo.

```powershell
.\build.ps1      # Windows
```

```sh
./build.sh       # Linux/macOS (cross-compilazione)
```

Entrambi eseguono `go vet` e tutti i test, poi creano `dist/ApkTool.exe` per Windows.
Per i soli test: `go test ./...`. Per compilare per il sistema in uso: `go build -o apktool .`.

## CI e release

- `.github/workflows/ci.yml`: a ogni push su `main` e a ogni pull request esegue `gofmt`, `go vet`
  e i test su Windows, Linux e macOS, più 30 secondi di fuzzing dei parser.
- `.github/workflows/release.yml`: quando si pubblica un tag `v*` esegue i test, compila per
  Windows (amd64), Linux (amd64, arm64) e macOS (Intel, Apple Silicon) e crea la release con
  gli archivi e il file `SHA256SUMS.txt`.

Per pubblicare una nuova versione:

```sh
git tag v1.0.0
git push origin v1.0.0
```

## Struttura

| Percorso | Contenuto |
|---|---|
| `main.go` | Flusso interattivo del programma |
| `names.go` | Validazione del package, nome del file di uscita, pulizia dei percorsi |
| `platform_windows.go` | Console UTF-8 e colori, input Unicode, finestra di selezione file, cartella Download |
| `platform_other.go` | Implementazioni per Linux e macOS (percorso digitato nel terminale), usate anche dai test |
| `internal/binres/` | Lettura e scrittura dell'XML binario Android e lettura di `resources.arsc` |
| `internal/manifest/` | Lettura e modifica di package ed etichetta nel manifest |
| `internal/apksign/` | Firma v1 (JAR) e v2, verifica v2, chiave di debug incorporata |
| `internal/apk/` | Lettura dell'APK e creazione della copia allineata e firmata |
| `testdata/` | Piccoli APK di prova usati dai test |
| `build.ps1`, `build.sh` | Test e compilazione dell'exe per Windows |
| `.github/workflows/` | CI e release automatiche |
| `rsrc_windows_amd64.syso` | Risorse Windows (icona, manifest, info versione) incluse automaticamente da `go build` |
| `tools/icon/`, `assets/` | Script Python (Pillow) che generano l'icona dell'exe e il file `.syso` |

Per rigenerare icona e risorse:

```sh
python tools/icon/make_icon.py assets/ApkTool.ico assets/icon_preview.png
python tools/icon/make_syso.py assets/ApkTool.ico rsrc_windows_amd64.syso
```
