// android-sim runs the Android build's Go engine on a desktop so the phone UI
// can be tested in a browser (use the browser's device emulation):
//
//	cp -r frontend/dist/. mobile/dist/ && go run ./cmd/android-sim
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	admmobile "github.com/StrStark/advance-download-manager/mobile"
)

func main() {
	base, _ := os.MkdirTemp("", "adm-android-sim-")
	url, err := admmobile.Start(filepath.Join(base, "data"), filepath.Join(base, "Download", "ADM"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Open:", url)
	fmt.Println("Files:", base)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	admmobile.Stop()
}
