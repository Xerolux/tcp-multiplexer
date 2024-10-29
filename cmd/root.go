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
	"github.com/spf13/cobra"
	"github.com/sirupsen/logrus"
	"os"
)

var (
	verbose        bool
	debug          bool
	configFilePath string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "tcp-multiplexer",
	Short: "A TCP multiplexer with flexible configuration and logging options",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	// Load configuration file if specified
	rootCmd.PersistentFlags().StringVarP(&configFilePath, "config", "c", "", "path to configuration file (JSON format)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose logging")
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "enable debug logging")

	cobra.OnInitialize(initConfig)
}

func initConfig() {
	// Set log formatting
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	// Set log level based on flags
	if debug {
		logrus.SetLevel(logrus.DebugLevel)
		logrus.SetReportCaller(true)
	} else if verbose {
		logrus.SetLevel(logrus.InfoLevel)
	} else {
		logrus.SetLevel(logrus.WarnLevel)
	}

	// Load config from file if provided
	if configFilePath != "" {
		if err := loadConfig(configFilePath); err != nil {
			logrus.Fatalf("Failed to load config file: %v", err)
		}
	}
}

// loadConfig loads configuration from the specified file path
func loadConfig(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("could not open config file: %w", err)
	}
	defer file.Close()

	// Assuming configuration includes fields like port, targetServer, and applicationProtocol
	var config struct {
		Port                string `json:"port"`
		TargetServer        string `json:"target_server"`
		ApplicationProtocol string `json:"application_protocol"`
	}

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return fmt.Errorf("could not decode config file: %w", err)
	}

	// Assign loaded config to the respective variables
	// Assuming `port`, `targetServer`, and `applicationProtocol` are used globally
	port = config.Port
	targetServer = config.TargetServer
	applicationProtocol = config.ApplicationProtocol

	logrus.Infof("Configuration loaded from %s", filePath)
	return nil
}
