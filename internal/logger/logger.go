package logger

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

var (
	Info  *log.Logger
	Error *log.Logger
)

// Init initializes the logger to write to both stdout and a file
func Init(logDir string) error {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	logFile, err := os.OpenFile(filepath.Join(logDir, "dbbridge.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return err
	}

	multiWriter := io.MultiWriter(os.Stdout, logFile)

	Info = log.New(multiWriter, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	Error = log.New(multiWriter, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)

	return nil
}
