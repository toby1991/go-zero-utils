package pagination

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakePaginationQuery struct {
	page     uint64
	pageSize uint64
}

func (q fakePaginationQuery) GetPage() uint64 {
	return q.page
}

func (q fakePaginationQuery) GetPageSize() uint64 {
	return q.pageSize
}

func TestValidatePaginationQuery(t *testing.T) {
	type fields struct{}
	type args struct {
		query     PaginationQuery
		returnAll bool
	}
	tests := []struct {
		name     string
		fields   fields
		args     args
		want     ValidatedPaginationQuery
		wantCode codes.Code
	}{
		{
			name:     "return all ignores nil query",
			args:     args{returnAll: true},
			want:     ValidatedPaginationQuery{Page: 1, PageSize: 0, Offset: 0, Limit: 0, ReturnAll: true},
			wantCode: codes.OK,
		},
		{
			name:     "return all ignores invalid query",
			args:     args{query: fakePaginationQuery{page: 0, pageSize: 0}, returnAll: true},
			want:     ValidatedPaginationQuery{Page: 1, PageSize: 0, Offset: 0, Limit: 0, ReturnAll: true},
			wantCode: codes.OK,
		},
		{
			name:     "missing query",
			args:     args{},
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "page must be positive",
			args:     args{query: fakePaginationQuery{page: 0, pageSize: 20}},
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "page size must be positive",
			args:     args{query: fakePaginationQuery{page: 1, pageSize: 0}},
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "page size must be less than fifty",
			args:     args{query: fakePaginationQuery{page: 1, pageSize: 50}},
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "valid first page",
			args:     args{query: fakePaginationQuery{page: 1, pageSize: 20}},
			want:     ValidatedPaginationQuery{Page: 1, PageSize: 20, Offset: 0, Limit: 20},
			wantCode: codes.OK,
		},
		{
			name:     "valid later page",
			args:     args{query: fakePaginationQuery{page: 3, pageSize: 15}},
			want:     ValidatedPaginationQuery{Page: 3, PageSize: 15, Offset: 30, Limit: 15},
			wantCode: codes.OK,
		},
		{
			name:     "offset overflow",
			args:     args{query: fakePaginationQuery{page: ^uint64(0), pageSize: 49}},
			wantCode: codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidatePaginationQuery(tt.args.query, tt.args.returnAll)
			if status.Code(err) != tt.wantCode {
				t.Fatalf("error code = %v, want %v, err = %v", status.Code(err), tt.wantCode, err)
			}
			if tt.wantCode != codes.OK {
				return
			}
			if got != tt.want {
				t.Fatalf("pagination query = %+v, want %+v", got, tt.want)
			}
		})
	}
}
