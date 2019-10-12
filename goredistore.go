// Adapted for use with go-redis.
// Copyright 2019, Allen Woods.
// All rights reserved.
//
// Adapted from:
// https://github.com/boj/redistore
// Copyright 2012, Brian "bojo" Jones.
// All rights reserved.
//
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goredistore

import (
	"bytes"
	"encoding/base32"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-redis/redis"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
)

// Amount of time for cookies/redis keys to expire.
var sessionExpire = 86400 * 30

// SessionSerializer provides an interface hook for alternative serializers
type SessionSerializer interface {
	Deserialize(d []byte, ss *sessions.Session) error
	Serialize(ss *sessions.Session) ([]byte, error)
}

// JSONSerializer encode the session map to JSON.
type JSONSerializer struct{}

// Serialize to JSON. Will err if there are unmarshalable key values
func (s JSONSerializer) Serialize(ss *sessions.Session) ([]byte, error) {
	m := make(map[string]interface{}, len(ss.Values))
	for k, v := range ss.Values {
		ks, ok := k.(string)
		if !ok {
			err := fmt.Errorf("Non-string key value, cannot serialize session to JSON: %v", k)
			fmt.Printf("go-redistore.JSONSerializer.serialize() Error: %v", err)
			return nil, err
		}
		m[ks] = v
	}
	return json.Marshal(m)
}

// Deserialize back to map[string]interface{}
func (s JSONSerializer) Deserialize(d []byte, ss *sessions.Session) error {
	m := make(map[string]interface{})
	err := json.Unmarshal(d, &m)
	if err != nil {
		fmt.Printf("redistore.JSONSerializer.deserialize() Error: %v", err)
		return err
	}
	for k, v := range m {
		ss.Values[k] = v
	}
	return nil
}

// GobSerializer uses gob package to encode the session map
type GobSerializer struct{}

// Serialize using gob
func (s GobSerializer) Serialize(ss *sessions.Session) ([]byte, error) {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	err := enc.Encode(ss.Values)
	if err == nil {
		return buf.Bytes(), nil
	}
	return nil, err
}

// Deserialize back to map[interface{}]interface{}
func (s GobSerializer) Deserialize(d []byte, ss *sessions.Session) error {
	dec := gob.NewDecoder(bytes.NewBuffer(d))
	return dec.Decode(&ss.Values)
}

// RediStore stores sessions in a redis backend.
type GoRediStore struct {
	Client        *redis.Client
	Codecs        []securecookie.Codec
	Options       *sessions.Options // default configuration
	DefaultMaxAge int               // default Redis TTL for a MaxAge == 0 session
	maxLength     int
	keyPrefix     string
	serializer    SessionSerializer
}

// SetMaxLength sets RediStore.maxLength if the `l` argument is greater or equal 0
// maxLength restricts the maximum length of new sessions to l.
// If l is 0 there is no limit to the size of a session, use with caution.
// The default for a new RediStore is 4096. Redis allows for max.
// value sizes of up to 512MB (http://redis.io/topics/data-types)
// Default: 4096,
func (s *GoRediStore) SetMaxLength(l int) {
	if l >= 0 {
		s.maxLength = l
	}
}

// SetKeyPrefix set the prefix
func (s *GoRediStore) SetKeyPrefix(p string) {
	s.keyPrefix = p
}

// SetSerializer sets the serializer
func (s *GoRediStore) SetSerializer(ss SessionSerializer) {
	s.serializer = ss
}

// SetMaxAge restricts the maximum age, in seconds, of the session record
// both in database and a browser. This is to change session storage configuration.
// If you want just to remove session use your session `s` object and change it's
// `Options.MaxAge` to -1, as specified in
//    http://godoc.org/github.com/gorilla/sessions#Options
//
// Default is the one provided by this package value - `sessionExpire`.
// Set it to 0 for no restriction.
// Because we use `MaxAge` also in SecureCookie crypting algorithm you should
// use this function to change `MaxAge` value.
func (s *GoRediStore) SetMaxAge(v int) {
	var c *securecookie.SecureCookie
	var ok bool
	s.Options.MaxAge = v
	for i := range s.Codecs {
		if c, ok = s.Codecs[i].(*securecookie.SecureCookie); ok {
			c.MaxAge(v)
		} else {
			fmt.Printf("Can't change MaxAge on codec %v\n", s.Codecs[i])
		}
	}
}

// NewRediStore returns a new RediStore.
// size: maximum number of idle connections.
func NewRediStore(size int, network, address, password string, keyPairs ...[]byte) (*GoRediStore, error) {

	// TestOnBorrow: func(c redis.Conn, t time.Time) error {
	// 	_, err := c.Do("PING").Result()
	// 	return err
	// },

	c := redis.NewClient(&redis.Options{
		Network:     network,
		Addr:        address,
		PoolSize:    size,
		IdleTimeout: 240 * time.Second,
		OnConnect: func(c *redis.Conn) error {
			return c.ClientGetName().Err()
		},
		Password: password,
	})

	return NewRediStoreWithPool(c, keyPairs...)
}

// NewRediStoreWithDB - like NewRedisStore but accepts `DB` parameter to select
// redis DB instead of using the default one ("0")
func NewRediStoreWithDB(size int, network, address, password string, DB int, keyPairs ...[]byte) (*GoRediStore, error) {
	c := redis.NewClient(&redis.Options{
		Network:     network,
		Addr:        address,
		PoolSize:    size,
		IdleTimeout: 240 * time.Second,
		OnConnect: func(c *redis.Conn) error {
			return c.ClientGetName().Err()
		},
		Password: password,
		DB:       DB,
	})
	return NewRediStoreWithPool(c, keyPairs...)
}

// NewRediStoreWithPool instantiates a RediStore with a *redis.Pool passed in.
func NewRediStoreWithPool(client *redis.Client, keyPairs ...[]byte) (*GoRediStore, error) {
	rs := &GoRediStore{
		// https://godoc.org/github.com/go-redis/redis#Client
		Client: client,
		Codecs: securecookie.CodecsFromPairs(keyPairs...),
		Options: &sessions.Options{
			Path:   "/",
			MaxAge: sessionExpire,
		},
		DefaultMaxAge: 60 * 20, // 20 minutes is a reasonable default
		maxLength:     4096,
		keyPrefix:     "session_",
		serializer:    GobSerializer{},
	}
	_, err := rs.ping()
	return rs, err
}

// Close closes the underlying *redis.Pool
func (s *GoRediStore) Close() error {
	return s.Client.Close()
}

// Get returns a session for the given name after adding it to the registry.
//
// See gorilla/sessions FilesystemStore.Get().
func (s *GoRediStore) Get(r *http.Request, name string) (*sessions.Session, error) {
	return sessions.GetRegistry(r).Get(s, name)
}

// New returns a session for the given name without adding it to the registry.
//
// See gorilla/sessions FilesystemStore.New().
func (s *GoRediStore) New(r *http.Request, name string) (*sessions.Session, error) {
	var (
		err error
		ok  bool
	)
	session := sessions.NewSession(s, name)
	// make a copy
	options := *s.Options
	session.Options = &options
	session.IsNew = true
	if c, errCookie := r.Cookie(name); errCookie == nil {
		err = securecookie.DecodeMulti(name, c.Value, &session.ID, s.Codecs...)
		if err == nil {
			ok, err = s.load(session)
			session.IsNew = !(err == nil && ok) // not new if no error and data available
		}
	}
	return session, err
}

// Save adds a single session to the response.
func (s *GoRediStore) Save(r *http.Request, w http.ResponseWriter, session *sessions.Session) error {
	// Marked for deletion.
	if session.Options.MaxAge <= 0 {
		if err := s.delete(session); err != nil {
			return err
		}
		http.SetCookie(w, sessions.NewCookie(session.Name(), "", session.Options))
	} else {
		// Build an alphanumeric key for the redis store.
		if session.ID == "" {
			session.ID = strings.TrimRight(base32.StdEncoding.EncodeToString(securecookie.GenerateRandomKey(32)), "=")
		}
		if err := s.save(session); err != nil {
			return err
		}
		encoded, err := securecookie.EncodeMulti(session.Name(), session.ID, s.Codecs...)
		if err != nil {
			return err
		}
		http.SetCookie(w, sessions.NewCookie(session.Name(), encoded, session.Options))
	}
	return nil
}

// Delete removes the session from redis, and sets the cookie to expire.
//
// WARNING: This method should be considered deprecated since it is not exposed via the gorilla/sessions interface.
// Set session.Options.MaxAge = -1 and call Save instead. - July 18th, 2013
func (s *GoRediStore) Delete(r *http.Request, w http.ResponseWriter, session *sessions.Session) error {
	conn := s.Client
	defer conn.Close()
	if _, err := conn.Do("DEL", s.keyPrefix+session.ID).Result(); err != nil {
		return err
	}
	// Set cookie to expire.
	options := *session.Options
	options.MaxAge = -1
	http.SetCookie(w, sessions.NewCookie(session.Name(), "", &options))
	// Clear session values.
	for k := range session.Values {
		delete(session.Values, k)
	}
	return nil
}

// ping does an internal ping against a server to check if it is alive.
func (s *GoRediStore) ping() (bool, error) {
	conn := s.Client
	defer conn.Close()
	data, err := conn.Do("PING").Result()
	if err != nil || data == nil {
		return false, err
	}
	return (data == "PONG"), nil
}

// save stores the session in redis.
func (s *GoRediStore) save(session *sessions.Session) error {
	b, err := s.serializer.Serialize(session)
	if err != nil {
		return err
	}
	if s.maxLength != 0 && len(b) > s.maxLength {
		return errors.New("SessionStore: the value to store is too big")
	}
	conn := s.Client
	defer conn.Close()
	if err = conn.ClientGetName().Err(); err != nil {
		return err
	}
	age := session.Options.MaxAge
	if age == 0 {
		age = s.DefaultMaxAge
	}
	_, err = conn.Do("SETEX", s.keyPrefix+session.ID, age, b).Result()
	return err
}

// load reads the session from redis.
// returns true if there is a sessoin data in DB
func (s *GoRediStore) load(session *sessions.Session) (bool, error) {
	conn := s.Client
	defer conn.Close()
	if err := conn.ClientGetName().Err(); err != nil {
		return false, err
	}
	data, err := conn.Do("GET", s.keyPrefix+session.ID).String()
	if err != nil {
		return false, err
	}
	if data == "" {
		return false, nil // no data was associated with this key
	}

	b := []byte(data)

	return true, s.serializer.Deserialize(b, session)
}

// delete removes keys from redis if MaxAge<0
func (s *GoRediStore) delete(session *sessions.Session) error {
	conn := s.Client
	defer conn.Close()
	if _, err := conn.Do("DEL", s.keyPrefix+session.ID).Result(); err != nil {
		return err
	}
	return nil
}
