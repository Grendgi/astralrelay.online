// Package logjson provides structured JSON logging (one JSON object per line).
package logjson

import (
	"encoding/json"
	"log"
)

// Log emits a structured log line: {"type": typ, ...fields}
func Log(typ string, fields map[string]interface{}) {
	m := map[string]interface{}{"type": typ}
	for k, v := range fields {
		m[k] = v
	}
	data, _ := json.Marshal(m)
	log.Printf("%s", string(data))
}
