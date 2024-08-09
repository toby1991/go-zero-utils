package bizmemory

type BizMemoryConf struct {
	DefaultExpirationMinute uint   `json:",default=60"`
	CleanUpIntervalMinute   uint   `json:",default=60"`
	Prefix                  string `json:",optional"`
}
