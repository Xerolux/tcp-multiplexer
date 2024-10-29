/*
Copyright © 2021 xujiahua <littleguner@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/ingmarstein/tcp-multiplexer/pkg/message"
	"github.com/ingmarstein/tcp-multiplexer/pkg/multiplexer"
	"github.com/sirupsen/logrus"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

var (
	port                string
	targetServer        string
	applicationProtocol string
	configFilePath      string
)

// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "start multiplexer proxy server",
	Run: func(cmd *cobra.Command, args []string) {
		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
		})
		logrus.SetLevel(logrus.WarnLevel)
		if verbose {
			logrus.SetLevel(logrus.InfoLevel)
		}
		if debug {
			logrus.SetReportCaller(true)
			logrus.SetLevel(logrus.DebugLevel)
		}

		// Load configuration from file if specified
		if configFilePath != "" {
			if err := loadConfig(configFilePath); err != nil {
				logrus.Fatalf("Failed to load config file: %v", err)
			}
		}

		// Validate application protocol
		msgReader, ok := message.Readers[applicationProtocol]
		if !ok {
			logrus.Errorf("%s application protocol is not supported", applicationProtocol)
			os.Exit(2)
		}

		mux := multiplexer.New(targetServer, port, msgReader)
		go func() {
			if err := mux.Start(); err != nil {
				logrus.Error(err)
				os.Exit(2)
			}
		}()

		signalChan := make(chan os.Signal, 1)
		signal.Notify(
			signalChan,
			syscall.SIGHUP,  // kill -SIGHUP XXXX
			syscall.SIGINT,  // kill -SIGINT XXXX or Ctrl+c
			syscall.SIGQUIT, // kill -SIGQUIT XXXX
		)
		<-signalChan

		if err := mux.Close(); err != nil {
			logrus.Error(err)
		}
	},
}

func loadConfig(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&struct {
		Port                string `json:"port"`
		TargetServer        string `json:"target_server"`
		ApplicationProtocol string `json:"application_protocol"`
	}{
		&port,
		&targetServer,
		&applicationProtocol,
	}); err != nil {
		return err
	}
	return nil
}

func init() {
	rootCmd.AddCommand(serverCmd)

	serverCmd.Flags().StringVarP(&port, "listen", "l", "8000", "multiplexer will listen on")
	serverCmd.Flags().StringVarP(&targetServer, "targetServer", "t", "127.0.0.1:1234", "multiplexer will forward message to")
	serverCmd.Flags().StringVarP(&applicationProtocol, "applicationProtocol", "p", "echo", "multiplexer will parse to message echo/http/iso8583/modbus")
	serverCmd.Flags().StringVarP(&configFilePath, "config", "c", "", "path to configuration file (JSON format)")
}
