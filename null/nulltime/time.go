package nulltime

import (
	"gopkg.in/guregu/null.v4"
	"time"
)

func Unix(t *time.Time) uint64 {
	//uint64(null.TimeFromPtr(v.Edges.Goods.Edges.Info.DeletedAt).ValueOrZero().Unix())
	tData := null.TimeFromPtr(t)

	if !tData.Valid {
		return 0
	}

	if tData.IsZero() {
		return 0
	}

	return uint64(tData.ValueOrZero().Unix())
}
