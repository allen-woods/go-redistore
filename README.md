# go-redistore

Adaptation of the redistore package that implements go-redis for use with gorilla/sessions.

# Analysis of `redigo`

In order to port this over to `go-redis`, all values of type, methods, and return values must be accurately analyzed.

All code is analyzed from top to bottom in chronological order of appearance.

<table>
  <thead>
    <tr>
      <th>
        <center>
          <strong>redigo</strong>
        <center>
      </th>
      <th>
        <center>
          <strong>go-redis</strong>
        </center>
      </th>
    </thead>
    <tbody>
      <tr>
        <td valign="top">
        <pre>
type Pool struct {
  Dial func() (Conn, error)
  DialContext func(ctx context.Context) (Conn, error)
  TestOnBorrow func(c Conn, t time.Time) error
  MaxIdle int
  MaxActive int
  IdleTimeout time.Duration
  Wait bool
  MaxConnLifetime time.Duration
}
        </pre>
        </td>
        <td valign="top">
        <pre>
type Options struct {
  Network string
  Addr string
  Dialer func(ctx context.Context, network, addr string) (net.Conn, error)
  OnConnect func(*Conn) error
  Password string
  DB int
  MaxRetries int
  MinRetryBackoff time.Duration
  MaxRetryBackoff time.Duration
  DialTimeout time.Duration
  ReadTimeout time.Duration
  WriteTimeout time.Duration
  PoolSize int
  MinIdleConns int
  MaxConnAge time.Duration
  PoolTimeout time.Duration
  IdleTimeout time.Duration
  IdleCheckFrequency time.Duration
  TLSConfig *tls.Config
}
          </pre>
        </td>
      </tr>
    </tbody>
  </table>
