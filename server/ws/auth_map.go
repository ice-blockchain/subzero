package ws

import (
	"hash/maphash"

	"github.com/llxisdsh/pb"
)

var (
	_ syncMap[Writer, connAuthData] = (*authMap)(nil)
)

type authMap struct {
	M    *pb.MapOf[uint64, connAuthData]
	Seed maphash.Seed
}

func newAuthMap() *authMap {
	return &authMap{
		Seed: maphash.MakeSeed(),
		M:    pb.NewMapOf[uint64, connAuthData](),
	}
}

func (m *authMap) hash(w Writer) uint64 {
	return maphash.Comparable(m.Seed, w)
}

func (m *authMap) Store(w Writer, data connAuthData) {
	m.M.Store(m.hash(w), data)
}

func (m *authMap) LoadOrCompute(w Writer, f func() (connAuthData, bool)) (connAuthData, bool) {
	return m.M.LoadOrCompute(m.hash(w), f)
}

func (m *authMap) Load(w Writer) (connAuthData, bool) {
	return m.M.Load(m.hash(w))
}

func (m *authMap) Delete(w Writer) {
	m.M.Delete(m.hash(w))
}

func (m *authMap) LoadAndStore(w Writer, value connAuthData) (connAuthData, bool) {
	return m.M.LoadAndStore(m.hash(w), value)
}

func (m *authMap) LoadAndDelete(w Writer) (connAuthData, bool) {
	return m.M.LoadAndDelete(m.hash(w))
}

func (*authMap) Range(func(Writer, connAuthData) bool) {
	panic("should not be used")
}
