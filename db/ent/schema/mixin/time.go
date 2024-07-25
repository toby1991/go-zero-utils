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
type TimeMixin struct {
	// We embed the `mixin.Schema` to avoid
	// implementing the rest of the methods.
	mixin.Schema
	InterceptorNewQueryFunc any
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
				// 如果查询类型实现了 WhereP 方法，则使用它
				d.P(query)
			default:
				// 如果查询类型没有实现 WhereP 方法，可以尝试其他方法
				// 例如，使用通用的 Where 方法（如果存在）
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

// Hooks of the SoftDeleteMixin.
func (d TimeMixin) Hooks() []ent.Hook {
	return []ent.Hook{
		On(
			func(next ent.Mutator) ent.Mutator {
				return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
					// Skip soft-delete, means delete the entity permanently.
					if skip, _ := ctx.Value(withSoftDeletedKey{}).(bool); skip {
						return next.Mutate(ctx, m)
					}
					//
					//// check if delete op
					//if !m.Op().Is(ent.OpDelete | ent.OpDeleteOne) {
					//	return next.Mutate(ctx, m)
					//}

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
					return next.Mutate(ctx, m)
				})
			},
			ent.OpDeleteOne|ent.OpDelete,
		),
	}
}

// P adds a storage-level predicate to the queries and mutations.
func (d TimeMixin) P(w interface{ WhereP(...func(*sql.Selector)) }) {
	w.WhereP(
		sql.FieldIsNull("deleted_at"),
	)
}

// If executes the given hook under condition.
//
//	hook.If(ComputeAverage, And(HasFields(...), HasAddedFields(...)))
func If(hk ent.Hook, cond Condition) ent.Hook {
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if cond(ctx, m) {
				return hk(next).Mutate(ctx, m)
			}
			return next.Mutate(ctx, m)
		})
	}
}

// On executes the given hook only for the given operation.
//
//	hook.On(Log, ent.Delete|ent.Create)
func On(hk ent.Hook, op ent.Op) ent.Hook {
	return If(hk, HasOp(op))
}

// Condition is a hook condition function.
type Condition func(context.Context, ent.Mutation) bool

// And groups conditions with the AND operator.
func And(first, second Condition, rest ...Condition) Condition {
	return func(ctx context.Context, m ent.Mutation) bool {
		if !first(ctx, m) || !second(ctx, m) {
			return false
		}
		for _, cond := range rest {
			if !cond(ctx, m) {
				return false
			}
		}
		return true
	}
}

// Or groups conditions with the OR operator.
func Or(first, second Condition, rest ...Condition) Condition {
	return func(ctx context.Context, m ent.Mutation) bool {
		if first(ctx, m) || second(ctx, m) {
			return true
		}
		for _, cond := range rest {
			if cond(ctx, m) {
				return true
			}
		}
		return false
	}
}

// Not negates a given condition.
func Not(cond Condition) Condition {
	return func(ctx context.Context, m ent.Mutation) bool {
		return !cond(ctx, m)
	}
}

// HasOp is a condition testing mutation operation.
func HasOp(op ent.Op) Condition {
	return func(_ context.Context, m ent.Mutation) bool {
		return m.Op().Is(op)
	}
}

// HasAddedFields is a condition validating `.AddedField` on fields.
func HasAddedFields(field string, fields ...string) Condition {
	return func(_ context.Context, m ent.Mutation) bool {
		if _, exists := m.AddedField(field); !exists {
			return false
		}
		for _, field := range fields {
			if _, exists := m.AddedField(field); !exists {
				return false
			}
		}
		return true
	}
}

// HasClearedFields is a condition validating `.FieldCleared` on fields.
func HasClearedFields(field string, fields ...string) Condition {
	return func(_ context.Context, m ent.Mutation) bool {
		if exists := m.FieldCleared(field); !exists {
			return false
		}
		for _, field := range fields {
			if exists := m.FieldCleared(field); !exists {
				return false
			}
		}
		return true
	}
}

// HasFields is a condition validating `.Field` on fields.
func HasFields(field string, fields ...string) Condition {
	return func(_ context.Context, m ent.Mutation) bool {
		if _, exists := m.Field(field); !exists {
			return false
		}
		for _, field := range fields {
			if _, exists := m.Field(field); !exists {
				return false
			}
		}
		return true
	}
}
