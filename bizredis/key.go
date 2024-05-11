package bizredis

type key struct {
	raw    string
	prefix string
}

func NewKey(raw string, prefix string) *key {
	k := key{}
	k.prefix = prefix
	k.raw = raw
	return &k
}
func (k *key) Raw() string {
	return k.raw
}
func (k *key) Prefixed() string {
	return k.prefix + k.raw
}
