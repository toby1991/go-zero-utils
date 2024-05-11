package db

type DbConf struct {
	DriverName string
	Dsn        string
	Debug      bool `json:",default=false"`
}
