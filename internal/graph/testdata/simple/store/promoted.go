package store

import "os"

// Promoted methods: conn's Close reaches OpenCloser only through types
// embedding it, since conn has no Open.
type OpenCloser interface {
	Open()
	Close()
}

type conn struct{}

func (*conn) Close() {}

// wrapsConn gets Close from *conn; deeper from wrapsConn's *conn, two levels down.
type wrapsConn struct{ *conn }

func (wrapsConn) Open() {}

type deeper struct{ wrapsConn }

// shadowed's own Close wins over the embedded one.
type shadowed struct{ *conn }

func (shadowed) Open()  {}
func (shadowed) Close() {}

// osFile gets Name, Read and Write from os.File, outside the repo; not
// OpenCloser, since os.File's Close returns an error.
type osFile struct{ *os.File }

func (osFile) Open() {}

func closeAll(oc OpenCloser) { oc.Close() }
