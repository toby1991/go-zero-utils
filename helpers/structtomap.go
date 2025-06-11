package helpers

import "encoding/json"

func StructToMap(_struct interface{}) (map[string]interface{}, error) {
	jsonBytes, err := json.Marshal(_struct)
	if err != nil {
		return nil, err
	}

	var m map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &m); err != nil {
		return nil, err
	}
	
	return m, nil
}
