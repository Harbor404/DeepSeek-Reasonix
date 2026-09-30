package decision

import (
	"bytes"
	"encoding/json"
	"io"
)

func decode(args []byte) (request, *problem) {
	var req request
	if err := strictJSON(args, &req); err != nil {
		return req, err
	}
	return req, validate(req)
}

func strictJSON(args []byte, value any) *problem {
	if len(args) > 256*1024 {
		return &problem{Code: "decision.input_limit"}
	}
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.UseNumber()
	if err := uniqueJSON(dec, 0); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return &problem{Code: "decision.json_invalid"}
	}
	dec = json.NewDecoder(bytes.NewReader(args))
	dec.DisallowUnknownFields()
	if err := dec.Decode(value); err != nil {
		return &problem{Code: "decision.schema_invalid"}
	}
	return nil
}

func uniqueJSON(dec *json.Decoder, depth int) *problem {
	if depth > 16 {
		return &problem{Code: "decision.json_depth_limit"}
	}
	token, err := dec.Token()
	if err != nil {
		return &problem{Code: "decision.json_invalid"}
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	for dec.More() {
		if delim == '{' {
			key, err := dec.Token()
			if err != nil {
				return &problem{Code: "decision.json_invalid"}
			}
			name, ok := key.(string)
			if !ok {
				return &problem{Code: "decision.json_invalid"}
			}
			if seen[name] {
				return &problem{Code: "decision.duplicate_field", ID: name}
			}
			seen[name] = true
		}
		if err := uniqueJSON(dec, depth+1); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return &problem{Code: "decision.json_invalid"}
	}
	return nil
}
