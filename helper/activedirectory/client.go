package activedirectory

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/go-ldap/ldap"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/vault/helper/tlsutil"
	log "github.com/mgutz/logxi/v1"
	"golang.org/x/text/encoding/unicode"
	"net"
	"net/url"
	"strings"
)

func NewClient(conf *Configuration) Client {
	return &client{conf}
}

type Client interface {
	Search(baseDN string, filter string) ([]*ldap.Entry, error)

	UpdatePassword(baseDN string, filter string, newPassword string) error

	UpdateUsername(baseDN string, filter string, newUsername string) error
}

type client struct {
	conf *Configuration
}

func (c *client) Search(baseDN string, filter string) ([]*ldap.Entry, error) {

	req := &ldap.SearchRequest{
		BaseDN: baseDN,
		Scope:  2, // TODO ???
		Filter: filter,
	}

	conn, err := c.getConnection()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	result, err := conn.Search(req)
	if err != nil {
		return nil, err
	}

	return result.Entries, nil
}

func (c *client) UpdatePassword(baseDN string, filter string, newPassword string) error {

	// TODO - also what if they haven't bound or authenticated with a cert? how return that err?

	entries, err := c.Search(baseDN, filter)
	if err != nil {
		return err
	}
	if len(entries) != 1 {
		return fmt.Errorf("password filter of %s doesn't match just one entry: %s", filter, entries)
	}

	// Active Directory doesn't recognize the passwordModify method.
	// See https://github.com/go-ldap/ldap/issues/106
	// for further description, and for this workaround.
	utf16 := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)

	// According to the MS docs, the password needs to be enclosed in quotes.
	pwdEncoded, err := utf16.NewEncoder().String("\"" + newPassword + "\"")
	if err != nil {
		return err
	}

	passReq := &ldap.ModifyRequest{
		DN: entries[0].DN,
		ReplaceAttributes: []ldap.PartialAttribute{
			{"unicodePwd", []string{pwdEncoded}},
		},
	}

	conn, err := c.getConnection()
	if err != nil {
		return err
	}
	defer conn.Close()

	return conn.Modify(passReq)
}

func (c *client) UpdateUsername(baseDN string, filter string, newUsername string) error {

	entries, err := c.Search(baseDN, filter)
	if err != nil {
		return err
	}
	if len(entries) != 1 {
		return fmt.Errorf("update filter of %s doesn't match just one entry: %s", filter, entries)
	}

	modifyRequest := &ldap.ModifyRequest{
		DN: entries[0].DN,
		ReplaceAttributes: []ldap.PartialAttribute{ // TODO which attributes?
			{"givenName", []string{newUsername}}, // TODO escape with quotes? and only attribute to update?
		},
	}

	conn, err := c.getConnection()
	if err != nil {
		return err
	}
	defer conn.Close()

	return conn.Modify(modifyRequest)
}

func (c *client) getConnection() (*ldap.Conn, error) {

	var retErr *multierror.Error

	urls := strings.Split(c.conf.Url, ",")
	for _, uut := range urls {
		conn, err := c.connect(uut)
		if err != nil {
			retErr = multierror.Append(retErr, fmt.Errorf("error parsing url %q: %s", uut, err.Error()))
			continue
		}
		return conn, nil
	}

	log.Debug("ldap: errors connecting to some hosts: %s", retErr.Error())
	return nil, retErr
}

func (c *client) connect(uut string) (*ldap.Conn, error) {

	u, err := url.Parse(uut)
	if err != nil {
		return nil, err
	}

	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		// err intentionally ignored
		// fall back to using the parsed url's host
		host = u.Host
	}

	var tlsConfig *tls.Config

	switch u.Scheme {

	case "ldap":

		if port == "" {
			port = "389"
		}

		conn, err := ldap.Dial("tcp", net.JoinHostPort(host, port))
		if err != nil {
			return nil, err
		}

		if c.conf.StartTLS {

			tlsConfig, err = c.getTLSConfig(host)
			if err != nil {
				return nil, err
			}
			if err = conn.StartTLS(tlsConfig); err != nil {
				return nil, err
			}
		}
		return conn, nil

	case "ldaps":

		if port == "" {
			port = "636"
		}

		tlsConfig, err = c.getTLSConfig(host)
		if err != nil {
			return nil, err
		}

		conn, err := ldap.DialTLS("tcp", net.JoinHostPort(host, port), tlsConfig)
		if err != nil {
			return nil, err
		}
		return conn, nil

	default:
		return nil, fmt.Errorf("invalid LDAP scheme in url %q", net.JoinHostPort(host, port))
	}
}

func (c *client) getTLSConfig(host string) (*tls.Config, error) {

	tlsConfig := &tls.Config{
		ServerName: host,
	}

	if c.conf.TLSMinVersion != "" {
		tlsMinVersion, ok := tlsutil.TLSLookup[c.conf.TLSMinVersion]
		if !ok {
			return nil, fmt.Errorf("invalid 'tls_min_version' in config")
		}
		tlsConfig.MinVersion = tlsMinVersion
	}

	if c.conf.TLSMaxVersion != "" {
		tlsMaxVersion, ok := tlsutil.TLSLookup[c.conf.TLSMaxVersion]
		if !ok {
			return nil, fmt.Errorf("invalid 'tls_max_version' in config")
		}
		tlsConfig.MaxVersion = tlsMaxVersion
	}

	if c.conf.InsecureTLS {
		tlsConfig.InsecureSkipVerify = true
	}

	if c.conf.Certificate != "" {
		caPool := x509.NewCertPool()
		ok := caPool.AppendCertsFromPEM([]byte(c.conf.Certificate))
		if !ok {
			return nil, fmt.Errorf("could not append CA certificate")
		}
		tlsConfig.RootCAs = caPool
	}
	return tlsConfig, nil
}
