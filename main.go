// You can edit this code!
// Click here and start typing.
package main

import (
	//"bufio"
	"fmt"
	"net/http"
	"io"
	"os"
	"log"
)

func main() {
	resp, err := http.Get("http://www.microsoft.com")
	if err != nil {
		fmt.Printf("Could not get URL: %v\n", err)
		return
	}
	if resp.StatusCode >= 300 {
		fmt.Printf("Wrong HTTP Status: %v\n", resp.Status)
		return
	}

	//rdr := bufio.NewReader(resp.Body)
	respBody, err := io.ReadAll(resp.Body)
	//bodyString, err := rdr.ReadString('\n')

	if err != nil {
		fmt.Printf("Error reading body: %v\n", err)
		return
	}
	bodyString := string(respBody)

	fmt.Printf("Status: %v\nBody:\n%v\n", resp.Status, bodyString)

	file, err := os.Open("file.txt") // For read access.
	if err != nil {
		log.Fatal(err)
	}
	file.Write([]byte(bodyString))

	file.Close()

}
