package ldap

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/hashicorp/vault/helper/tlsutil"
	"github.com/hashicorp/vault/logical/framework"
	"strings"
	"text/template"
)

func NewConfiguration(fieldData *framework.FieldData) (*Configuration, error) {

	conf := &Configuration{}

	url := fieldData.Get("url").(string)
	if url != "" {
		conf.Url = strings.ToLower(url)
	}
	userattr := fieldData.Get("userattr").(string)
	if userattr != "" {
		conf.UserAttr = strings.ToLower(userattr)
	}
	userdn := fieldData.Get("userdn").(string)
	if userdn != "" {
		conf.UserDN = userdn
	}
	groupdn := fieldData.Get("groupdn").(string)
	if groupdn != "" {
		conf.GroupDN = groupdn
	}
	groupfilter := fieldData.Get("groupfilter").(string)
	if groupfilter != "" {
		// Validate the template before proceeding
		_, err := template.New("queryTemplate").Parse(groupfilter)
		if err != nil {
			return nil, fmt.Errorf("invalid groupfilter (%v)", err)
		}

		conf.GroupFilter = groupfilter
	}
	groupattr := fieldData.Get("groupattr").(string)
	if groupattr != "" {
		conf.GroupAttr = groupattr
	}
	upndomain := fieldData.Get("upndomain").(string)
	if upndomain != "" {
		conf.UPNDomain = upndomain
	}
	certificate := fieldData.Get("certificate").(string)
	if certificate != "" {
		block, _ := pem.Decode([]byte(certificate))

		if block == nil || block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("failed to decode PEM block in the certificate")
		}
		_, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse certificate %s", err.Error())
		}
		conf.Certificate = certificate
	}
	insecureTLS := fieldData.Get("insecure_tls").(bool)
	if insecureTLS {
		conf.InsecureTLS = insecureTLS
	}
	conf.TLSMinVersion = fieldData.Get("tls_min_version").(string)
	if conf.TLSMinVersion == "" {
		return nil, fmt.Errorf("failed to get 'tls_min_version' value")
	}

	var ok bool
	_, ok = tlsutil.TLSLookup[conf.TLSMinVersion]
	if !ok {
		return nil, fmt.Errorf("invalid 'tls_min_version'")
	}

	conf.TLSMaxVersion = fieldData.Get("tls_max_version").(string)
	if conf.TLSMaxVersion == "" {
		return nil, fmt.Errorf("failed to get 'tls_max_version' value")
	}

	_, ok = tlsutil.TLSLookup[conf.TLSMaxVersion]
	if !ok {
		return nil, fmt.Errorf("invalid 'tls_max_version'")
	}
	if conf.TLSMaxVersion < conf.TLSMinVersion {
		return nil, fmt.Errorf("'tls_max_version' must be greater than or equal to 'tls_min_version'")
	}

	startTLS := fieldData.Get("starttls").(bool)
	if startTLS {
		conf.StartTLS = startTLS
	}
	bindDN := fieldData.Get("binddn").(string)
	if bindDN != "" {
		conf.BindDN = bindDN
	}
	bindPass := fieldData.Get("bindpass").(string)
	if bindPass != "" {
		conf.BindPassword = bindPass
	}
	denyNullBind := fieldData.Get("deny_null_bind").(bool)
	if denyNullBind {
		conf.DenyNullBind = denyNullBind
	}
	discoverDN := fieldData.Get("discoverdn").(bool)
	if discoverDN {
		conf.DiscoverDN = discoverDN
	}
	return conf, nil
}

type Configuration struct {
	Url           string `json:"url" structs:"url" mapstructure:"url"`
	UserDN        string `json:"userdn" structs:"userdn" mapstructure:"userdn"`
	GroupDN       string `json:"groupdn" structs:"groupdn" mapstructure:"groupdn"`
	GroupFilter   string `json:"groupfilter" structs:"groupfilter" mapstructure:"groupfilter"`
	GroupAttr     string `json:"groupattr" structs:"groupattr" mapstructure:"groupattr"`
	UPNDomain     string `json:"upndomain" structs:"upndomain" mapstructure:"upndomain"`
	UserAttr      string `json:"userattr" structs:"userattr" mapstructure:"userattr"`
	Certificate   string `json:"certificate" structs:"certificate" mapstructure:"certificate"`
	InsecureTLS   bool   `json:"insecure_tls" structs:"insecure_tls" mapstructure:"insecure_tls"`
	StartTLS      bool   `json:"starttls" structs:"starttls" mapstructure:"starttls"`
	BindDN        string `json:"binddn" structs:"binddn" mapstructure:"binddn"`
	BindPassword  string `json:"bindpass" structs:"bindpass" mapstructure:"bindpass"`
	DenyNullBind  bool   `json:"deny_null_bind" structs:"deny_null_bind" mapstructure:"deny_null_bind"`
	DiscoverDN    bool   `json:"discoverdn" structs:"discoverdn" mapstructure:"discoverdn"`
	TLSMinVersion string `json:"tls_min_version" structs:"tls_min_version" mapstructure:"tls_min_version"`
	TLSMaxVersion string `json:"tls_max_version" structs:"tls_max_version" mapstructure:"tls_max_version"`
}
