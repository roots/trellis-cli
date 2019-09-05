package cmd

import (
	"io/ioutil"
	"log"
	"os"
)

func writeFile(destination string, content string) {
	contentByte := []byte(content)

	writeFileErr := ioutil.WriteFile(destination, contentByte, 0644)
	if writeFileErr != nil {
		log.Fatal(writeFileErr)
	}
}

func deleteFile(path string) {
	err := os.Remove(path)

	if err != nil {
		log.Fatal(err)
	}
}
