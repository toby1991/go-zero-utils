package pagination

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const MaxPageSize uint64 = 49

// PaginationQuery 是 go-zero RPC 标准分页请求需要满足的最小接口。
type PaginationQuery interface {
	GetPage() uint64
	GetPageSize() uint64
}

// Window 是已校验分页请求转换出的查询窗口。
type Window struct {
	Page      uint64
	PageSize  uint64
	Offset    int
	Limit     int
	ReturnAll bool
}

// ValidatePaginationQuery 校验 paginationQuery + returnAll 契约，并返回 Offset/Limit。
func ValidatePaginationQuery(query PaginationQuery, returnAll bool) (Window, error) {
	if returnAll {
		return Window{Page: 1, ReturnAll: true}, nil
	}
	if query == nil {
		return Window{}, status.Error(codes.InvalidArgument, "paginationQuery is required when returnAll is false")
	}

	page := query.GetPage()
	if page == 0 {
		return Window{}, status.Error(codes.InvalidArgument, "paginationQuery.page must be greater than 0")
	}
	pageSize := query.GetPageSize()
	if pageSize == 0 {
		return Window{}, status.Error(codes.InvalidArgument, "paginationQuery.pageSize must be greater than 0")
	}
	if pageSize > MaxPageSize {
		return Window{}, status.Error(codes.InvalidArgument, "paginationQuery.pageSize must be less than 50")
	}

	maxInt := uint64(^uint(0) >> 1)
	if page > 1 && page-1 > maxInt/pageSize {
		return Window{}, status.Error(codes.InvalidArgument, "paginationQuery offset overflows int")
	}

	return Window{
		Page:     page,
		PageSize: pageSize,
		Offset:   int((page - 1) * pageSize),
		Limit:    int(pageSize),
	}, nil
}
