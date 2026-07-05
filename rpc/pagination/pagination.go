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

// ValidatedPaginationQuery 是已校验分页请求转换出的分页查询参数。
type ValidatedPaginationQuery struct {
	Page      uint64
	PageSize  uint64
	Offset    int
	Limit     int
	ReturnAll bool
}

// ValidatePaginationQuery 校验 paginationQuery + returnAll 契约，并返回 Offset/Limit。
func ValidatePaginationQuery(query PaginationQuery, returnAll bool) (ValidatedPaginationQuery, error) {
	if returnAll {
		return ValidatedPaginationQuery{Page: 1, ReturnAll: true}, nil
	}
	if query == nil {
		return ValidatedPaginationQuery{}, status.Error(codes.InvalidArgument, "paginationQuery is required when returnAll is false")
	}

	page := query.GetPage()
	if page == 0 {
		return ValidatedPaginationQuery{}, status.Error(codes.InvalidArgument, "paginationQuery.page must be greater than 0")
	}
	pageSize := query.GetPageSize()
	if pageSize == 0 {
		return ValidatedPaginationQuery{}, status.Error(codes.InvalidArgument, "paginationQuery.pageSize must be greater than 0")
	}
	if pageSize > MaxPageSize {
		return ValidatedPaginationQuery{}, status.Error(codes.InvalidArgument, "paginationQuery.pageSize must be less than 50")
	}

	maxInt := uint64(^uint(0) >> 1)
	if page > 1 && page-1 > maxInt/pageSize {
		return ValidatedPaginationQuery{}, status.Error(codes.InvalidArgument, "paginationQuery offset overflows int")
	}

	return ValidatedPaginationQuery{
		Page:     page,
		PageSize: pageSize,
		Offset:   int((page - 1) * pageSize),
		Limit:    int(pageSize),
	}, nil
}
