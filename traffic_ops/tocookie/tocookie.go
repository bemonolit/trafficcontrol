// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

// http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tocookie

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/apache/incubator-trafficcontrol/traffic_monitor_golang/common/log"
)

const GeneratedByStr = "trafficcontrol-go-tocookie"
const Name = "mojolicious"
const DefaultDuration = time.Hour

type Cookie struct {
	AuthData    string `json:"auth_data"`
	ExpiresUnix int64  `json:"expires"`
	By          string `json:"by"`
}

func checkHmac(message, messageMAC, key []byte) bool {
	mac := hmac.New(sha1.New, key)
	mac.Write(message)
	expectedMAC := mac.Sum(nil)
	return hmac.Equal(messageMAC, expectedMAC)
}

func Parse(secret, cookie string) (*Cookie, error) {
	dashPos := strings.Index(cookie, "-")
	if dashPos == -1 {
		return nil, fmt.Errorf("malformed cookie '%s' - no dashes", cookie)
	}

	lastDashPos := strings.LastIndex(cookie, "-")
	if lastDashPos == -1 {
		return nil, fmt.Errorf("malformed cookie '%s' - no dashes", cookie)
	}

	if len(cookie) < lastDashPos+1 {
		return nil, fmt.Errorf("malformed cookie '%s' -- no signature", cookie)
	}

	base64Txt := cookie[:dashPos]
	txtBytes, err := base64.RawURLEncoding.DecodeString(base64Txt)
	if err != nil {
		return nil, fmt.Errorf("error decoding base64 data: %v", err)
	}
	base64TxtSig := cookie[:lastDashPos-1] // the signature signs the base64 including trailing hyphens, but the Go base64 decoder doesn't want the trailing hyphens.

	base64Sig := cookie[lastDashPos+1:]
	sigBytes, err := hex.DecodeString(base64Sig)
	if err != nil {
		return nil, fmt.Errorf("error decoding signature: %v", err)
	}

	if !checkHmac([]byte(base64TxtSig), sigBytes, []byte(secret)) {
		return nil, fmt.Errorf("bad signature")
	}

	cookieData := Cookie{}
	if err := json.Unmarshal(txtBytes, &cookieData); err != nil {
		return nil, fmt.Errorf("error decoding base64 text '%s' to JSON: %v", string(txtBytes), err)
	}

	if cookieData.ExpiresUnix-time.Now().Unix() < 0 {
		now := time.Now()
		then := time.Unix(cookieData.ExpiresUnix, 0)
		log.Errorf("signature expired: %s < %s", then.Format(time.RFC3339), now.Format(time.RFC3339))
		return nil, fmt.Errorf("signature expired")
	}

	return &cookieData, nil
}

func NewRawMsg(msg, key []byte) string {
	base64Msg := base64.RawURLEncoding.WithPadding('-').EncodeToString(msg)
	mac := hmac.New(sha1.New, []byte(key))
	mac.Write([]byte(base64Msg))
	encMac := mac.Sum(nil)
	base64Sig := hex.EncodeToString(encMac)
	return base64Msg + "--" + base64Sig
}

func New(user string, expiration time.Time, key string) string {
	cookieMsg := Cookie{By: GeneratedByStr, AuthData: user, ExpiresUnix: expiration.Unix()}
	msg, _ := json.Marshal(cookieMsg)
	return NewRawMsg(msg, []byte(key))
}

// Update takes an existing cookie and returns a new serialized cookie with an updated expiration
func Refresh(c *Cookie, key string) string {
	return New(c.AuthData, time.Now().Add(DefaultDuration), key)
}
