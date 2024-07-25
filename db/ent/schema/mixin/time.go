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

// Interceptors of the SoftDeleteMixin.
func (d TimeMixin) Interceptors() []ent.Interceptor {
	return []ent.Interceptor{
		ent.TraverseFunc(func(ctx context.Context, q ent.Query) error {
			// With soft-deleted, means include soft-deleted entities.
			if skip, _ := ctx.Value(withSoftDeletedKey{}).(bool); skip {
				return nil
			}

			qp, ok := q.(interface {
				WhereP(...func(*sql.Selector))
			})
			if !ok {
				return fmt.Errorf("unexpected query type %T", q)
			}
			d.P(qp)
			return nil
		}),
	}
}

// Hooks of the SoftDeleteMixin.
func (d TimeMixin) Hooks() []ent.Hook {
	return []ent.Hook{

		func(next ent.Mutator) ent.Mutator {
			return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
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
				return next.Mutate(ctx, m)
			})
		},
	}
}

// P adds a storage-level predicate to the queries and mutations.
func (d TimeMixin) P(w interface{ WhereP(...func(*sql.Selector)) }) {
	w.WhereP(
		sql.FieldIsNull("deleted_at"),
	)
}
