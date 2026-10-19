# Evitare l'avviso di Windows SmartScreen

## Perché compare

SmartScreen ("Windows ha protetto il PC") controlla gli eseguibili che hanno il
*Mark of the Web*, cioè il flag `Zone.Identifier` che browser, Outlook e Teams aggiungono ai file
scaricati. Se l'exe non è firmato, o il suo firmatario non ha ancora abbastanza reputazione presso
Microsoft, compare l'avviso. Un exe compilato sul PC stesso, o copiato da una cartella di rete o da
una chiavetta, di solito non ha quel flag e quindi non mostra l'avviso.

## Opzioni, dalla più semplice

| Opzione | Costo | Effetto |
|---|---|---|
| Distribuire l'exe senza download web (cartella di rete aziendale, chiavetta, compilazione locale con `build.ps1`) | 0 | Nessun avviso, perché manca il Mark of the Web |
| Togliere il flag una volta: Proprietà del file → "Annulla blocco", oppure `Unblock-File .\ApkTool.exe` in PowerShell | 0 | Nessun avviso su quel PC per quel file |
| PC gestiti dall'IT: consentire l'exe (hash o certificato) con Intune/AppLocker o gli indicatori di Defender for Endpoint | 0 | Nessun avviso sui PC aziendali |
| Inviare l'exe a Microsoft come falso positivo ([modulo di invio](https://www.microsoft.com/wdsi/filesubmission)) | 0 | Può sbloccare quella specifica build; va ripetuto a ogni nuova versione |
| **Azure Artifact Signing** (ex Trusted Signing), con l'azienda come intestataria | circa 10 $/mese | Firma l'exe col nome dell'azienda; la reputazione si accumula con i download e passa alle versioni successive |
| Certificato di firma OV da una CA | 200–400 €/anno, con chiave su token hardware | Come sopra |

Note verificate su [Microsoft Learn](https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/code-signing-options)
(ottobre 2026):

- Azure Artifact Signing è disponibile per **organizzazioni** in USA, Canada, UE e Regno Unito;
  per le **persone fisiche** solo in USA e Canada. Da un'azienda italiana si può usare.
- **Nessuna firma elimina l'avviso dal primo giorno**: dal 2024 anche i certificati EV non lo
  saltano più. La reputazione cresce man mano che l'exe firmato viene scaricato ed eseguito.
- Firmare ogni versione con la stessa identità fa sì che le nuove versioni ereditino la
  reputazione delle precedenti.

## Raccomandazione

- Se lo usano solo i colleghi: distribuire l'exe da una cartella di rete aziendale, oppure
  chiedere all'IT di consentirlo. È gratis, immediato e non richiede firme.
- Se va distribuito via web a persone esterne: Azure Artifact Signing intestato all'azienda.
  La firma si aggiunge a `build.ps1` con `signtool` e l'avviso sparisce col crescere della
  reputazione.
