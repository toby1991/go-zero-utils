package faktory

import (
	"fmt"
)

// 为兼容 event lisenter 逻辑，将 faktory 的 jobType 中也包含 queue
func ToJobType(jobType string, jobQueue string) string {
	return fmt.Sprintf("%s||%s", jobType, jobQueue)
}

//
//func ParseJobType(jobType string) (event string, listener string, err error) {
//	n := strings.SplitN(jobType, "||", 2)
//	if len(n) != 2 {
//
//		return "", "", errors.New("invalid job type")
//	}
//
//	return n[0], n[1], nil
//}
