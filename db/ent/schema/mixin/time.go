package mixin

import (
	"context"
	"entgo.io/ent"
	"entgo.io/ent/dialect/sql"
	"fmt"
	"time"

	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// -------------------------------------------------
// Mixin definition
// TimeMixin implements the ent.Mixin for sharing
// time fields with package schemas.

// Usage: After first `go generate ./ent`
// Import `"youproject.git/internal/ent/intercept"`
// Import `ent2 "youproject.git/internal/ent"`
// Add `mixin.TimeMixin{InterceptorNewQueryFunc: func(q ent.Query) (any, error) { return intercept.NewQuery(q) }, MutationMutatorFunc: func(mutation ent.Mutation) ent.Mutator {return mutation.(interface{ Client() *ent2.Client }).Client()}},`
// to each schema files

type TimeMixin struct {
	// We embed the `mixin.Schema` to avoid
	// implementing the rest of the methods.
	mixin.Schema
	InterceptorNewQueryFunc func(query ent.Query) (any, error)
	MutationMutatorFunc     func(mutation ent.Mutation) ent.Mutator
}

func (TimeMixin) Fields() []ent.Field {
	return []ent.Field{

		field.Time("created_at").Immutable().Default(time.Now).Comment("创建时间"),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).Comment("更新时间"),
		field.Time("deleted_at").Optional().Nillable().Comment("删除时间"),
	}
}

type withSoftDeletedKey struct{}

// WithSoftDeleted returns a new context that skips the soft-delete interceptor/mutators.
func WithSoftDeleted(parent context.Context) context.Context {
	return context.WithValue(parent, withSoftDeletedKey{}, true)
}

// The Query interface represents an operation that queries a graph.
// By using this interface, users can write generic code that manipulates
// query builders of different types.
type Query interface {
	// Type returns the string representation of the query type.
	Type() string
	// Limit the number of records to be returned by this query.
	Limit(int)
	// Offset to start from.
	Offset(int)
	// Unique configures the query builder to filter duplicate records.
	Unique(bool)
	// Order specifies how the records should be ordered.
	Order(...func(*sql.Selector))
	// WhereP appends storage-level predicates to the query builder. Using this method, users
	// can use type-assertion to append predicates that do not depend on any generated package.
	WhereP(...func(*sql.Selector))
}

type TraverseFunc struct {
	InterceptorNewQueryFunc func(query ent.Query) (any, error) //func(ent.Query) (Query, error)
	Interceptor             func(context.Context, Query) error
}

// Intercept is a dummy implementation of Intercept that returns the next Querier in the pipeline.
func (f TraverseFunc) Intercept(next ent.Querier) ent.Querier {
	return next
}

// Traverse calls f(ctx, q).
func (f TraverseFunc) Traverse(ctx context.Context, q ent.Query) error {
	query, err := f.InterceptorNewQueryFunc(q)
	if err != nil {
		return err
	}
	return f.Interceptor(ctx, query.(Query))
}

// Interceptors of the SoftDeleteMixin.
func (d TimeMixin) Interceptors() []ent.Interceptor {
	// not intercept
	if d.InterceptorNewQueryFunc == nil {
		return nil
	}

	traverseFunc := TraverseFunc{
		InterceptorNewQueryFunc: d.InterceptorNewQueryFunc,
		Interceptor: func(ctx context.Context, q Query) error {

			// With soft-deleted, means include soft-deleted entities.
			if skip, _ := ctx.Value(withSoftDeletedKey{}).(bool); skip {
				return nil
			}

			// check query type is interceptor.Query or ent.Query
			switch query := q.(type) {
			case interface{ WhereP(...func(*sql.Selector)) }:
				// if query has implement WhereP
				d.P(query)
			default:
				// or Where
				if whereQuery, ok := q.(interface{ Where(...func(*sql.Selector)) }); ok {
					whereQuery.Where(func(s *sql.Selector) {
						sql.FieldIsNull("deleted_at")
					})
				} else {
					// 如果既没有 WhereP 也没有 Where 方法，记录一个警告或错误
					return fmt.Errorf("warning: Query type %T does not implement WhereP or Where method", q)
				}
			}
			return nil
		},
	}

	return []ent.Interceptor{
		traverseFunc,
	}
}

type MutateFunc struct {
	MutationMutatorFunc func(m ent.Mutation) ent.Mutator                             //func(ent.Query) (Query, error)
	Mutator             func(ctx context.Context, m ent.Mutation) (ent.Value, error) // func(next ent.Mutator) ent.Mutator
}

func (m MutateFunc) Mutate(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
	// won't do anything
	return m.Mutator(ctx, mutation)
}

// Hooks of the SoftDeleteMixin.
func (d TimeMixin) Hooks() []ent.Hook {
	// not hook
	if d.MutationMutatorFunc == nil {
		return nil
	}

	return []ent.Hook{
		func(next ent.Mutator) ent.Mutator {
			mutateFunc := MutateFunc{
				MutationMutatorFunc: d.MutationMutatorFunc,
				Mutator: func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
					// Skip soft-delete, means delete the entity permanently.
					if skip, _ := ctx.Value(withSoftDeletedKey{}).(bool); skip {
						return next.Mutate(ctx, m)
					}

					// check if delete op
					if !m.Op().Is(ent.OpDelete | ent.OpDeleteOne) {
						return next.Mutate(ctx, m)
					}

					mx, ok := m.(interface {
						SetOp(ent.Op)
						SetDeletedAt(time.Time)
						WhereP(...func(*sql.Selector))
					})
					if !ok {
						return nil, fmt.Errorf("unexpected mutation type %T", m)
					}
					d.P(mx)
					mx.SetOp(ent.OpUpdate)
					mx.SetDeletedAt(time.Now())

					return d.MutationMutatorFunc(mx.(ent.Mutation)).Mutate(ctx, m)
				},
			}
			return mutateFunc
		},
	}
}

// P adds a storage-level predicate to the queries and mutations.
func (d TimeMixin) P(w interface{ WhereP(...func(*sql.Selector)) }) {
	w.WhereP(
		sql.FieldIsNull("deleted_at"),
	)
}
