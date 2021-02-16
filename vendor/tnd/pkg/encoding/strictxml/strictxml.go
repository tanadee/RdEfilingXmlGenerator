package strictxml

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
)

// FormatIndent xml without newline at the end of file
func FormatIndent(reader io.Reader, writer io.Writer, prefix string, indent string) error {
	scanner := bufio.NewScanner(reader)
	scanner.Split(func(data []byte, eof bool) (int, []byte, error) {
		index := bytes.IndexAny(data, "<>")
		if index >= 0 {
			return index + 1, data[:index], nil
		}
		if eof {
			if len(data) == 0 {
				return 0, nil, nil
			}
			return len(data), data, nil
		}
		return 0, nil, nil
	})
	indentLevel := 0
	isTag := true
	newLineOnNextTag := false
	if scanner.Scan() {
		if _, err := fmt.Fprint(writer, scanner.Text()); err != nil {
			return err
		}
	}
	for scanner.Scan() {
		token := scanner.Text()
		if isTag {
			if len(token) > 0 && token[0] == '/' {
				indentLevel--
			}
			if newLineOnNextTag {
				if _, err := fmt.Fprintln(writer); err != nil {
					return err
				}
				if _, err := fmt.Fprint(writer, prefix); err != nil {
					return err
				}
				for i := 0; i < indentLevel; i++ {
					if _, err := fmt.Fprint(writer, indent); err != nil {
						return err
					}
				}
			}
			if len(token) == 0 {
				indentLevel++
			} else if token[0] != '/' {
				indentLevel++
				if token[len(token)-1] == '/' {
					indentLevel--
				}
			}
			if _, err := fmt.Fprint(writer, "<", token, ">"); err != nil {
				return err
			}
		} else {
			if len(strings.TrimSpace(token)) != 0 {
				if _, err := fmt.Fprint(writer, token); err != nil {
					return err
				}
				newLineOnNextTag = false
			} else {
				newLineOnNextTag = true
			}
		}
		isTag = !isTag
	}
	return nil
}

func main() {
	FormatIndent(strings.NewReader("<QE><q/><e></e><></></QE><Q></QEQ>"), os.Stdout, "", "\t")
}
