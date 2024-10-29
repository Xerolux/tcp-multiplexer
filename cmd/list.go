/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"fmt"
	"github.com/ingmarstein/tcp-multiplexer/pkg/message"
	"github.com/spf13/cobra"
	"text/tabwriter"
	"os"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List application protocols supported by the multiplexer",
	Run: func(cmd *cobra.Command, args []string) {
		// Check if there are protocols available
		if len(message.Readers) == 0 {
			fmt.Println("No supported application protocols found.")
			return
		}

		// Tabwriter for formatted output
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "SUPPORTED PROTOCOLS\tDESCRIPTION")
		fmt.Fprintln(w, "-------------------\t-----------")

		// Print each supported protocol and description
		for name := range message.Readers {
			description := getProtocolDescription(name)
			fmt.Fprintf(w, "%s\t%s\n", name, description)
		}
		w.Flush()

		fmt.Println("\nUsage example:")
		fmt.Println("  ./tcp-multiplexer server -p <protocol>")
		fmt.Println("Replace <protocol> with one of the supported protocols listed above.")
	},
}

// getProtocolDescription provides a description for each supported protocol
func getProtocolDescription(protocol string) string {
	switch protocol {
	case "echo":
		return "Simple echo protocol for testing connectivity."
	case "http":
		return "Standard HTTP protocol parsing."
	case "iso8583":
		return "ISO 8583 protocol for financial transaction messages."
	case "modbus":
		return "Modbus protocol commonly used in industrial automation."
	default:
		return "No description available."
	}
}

func init() {
	rootCmd.AddCommand(listCmd)
}
