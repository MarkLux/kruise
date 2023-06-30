package util

import (
	"encoding/json"

	v1 "k8s.io/api/core/v1"
)

func GetMessageKvFromCondition(condition *v1.PodCondition) (map[string]interface{}, error) {
	messageKv := make(map[string]interface{})
	if condition.Message != "" {
		if err := json.Unmarshal([]byte(condition.Message), &messageKv); err != nil {
			return nil, err
		}
	}
	return messageKv, nil
}

func UpdateMessageKvCondition(kv map[string]interface{}, condition *v1.PodCondition) error {
	message, err := json.Marshal(kv)
	if err != nil {
		return err
	}
	condition.Message = string(message)
	return nil
}
