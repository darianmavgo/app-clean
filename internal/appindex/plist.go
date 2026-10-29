package appindex

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// PlistDict holds extracted key-value pairs from Info.plist
type PlistDict map[string]string

// ParsePlistFile parses an XML or binary Info.plist file.
func ParsePlistFile(path string) (PlistDict, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if bytes.HasPrefix(data, []byte("bplist00")) {
		cmd := exec.Command("/usr/bin/plutil", "-convert", "xml1", "-o", "-", path)
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("plutil conversion failed for %s: %w", path, err)
		}
		data = out
	}

	return parsePlistXML(data)
}

func parsePlistXML(data []byte) (PlistDict, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	dict := make(PlistDict)
	var currentKey string

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return dict, err
		}

		switch elem := token.(type) {
		case xml.StartElement:
			switch elem.Name.Local {
			case "key":
				var k string
				if err := decoder.DecodeElement(&k, &elem); err == nil {
					currentKey = strings.TrimSpace(k)
				}
			case "string":
				var val string
				if err := decoder.DecodeElement(&val, &elem); err == nil && currentKey != "" {
					dict[currentKey] = strings.TrimSpace(val)
					currentKey = ""
				}
			}
		}
	}
	return dict, nil
}
