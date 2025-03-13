package helpers

import "encoding/json"

func MapToStruct(m map[string]interface{}, result interface{}) error {
	jsonBytes, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(jsonBytes, result)
}
