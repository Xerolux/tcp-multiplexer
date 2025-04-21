package main

import (
	"fmt"
	"html"
	"net/http"
	"net/http/httputil"
	"os"
)

func getPort() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "1234"
	}
	return port
}

func headers(w http.ResponseWriter, req *http.Request) {
	dump, err := httputil.DumpRequest(req, true)
	if err != nil {
		fmt.Println(err)
		http.Error(w, "Failed to dump request", http.StatusInternalServerError)
		return
	}
	fmt.Println(string(dump))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	
	// Fehlerrückgabewert von Write überprüfen
	_, err = w.Write([]byte(html.EscapeString(string(dump))))
	if err != nil {
		fmt.Printf("Error writing response: %v\n", err)
		// Da wir bereits mit dem Schreiben begonnen haben, können wir http.Error nicht mehr verwenden
		// Wir können nur den Fehler loggen
	}
}

func main() {
	http.HandleFunc("/", headers)

	port := getPort()
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		fmt.Printf("Error listening on port %s: %v\n", port, err)
	}
}
