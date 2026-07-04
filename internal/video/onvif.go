package video

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Crypto Helpers

// EncryptPassword encrypts a plain password using AES-GCM and base64 encodes the result
func EncryptPassword(password string, secretKey []byte) (string, error) {
	derivedKey := make([]byte, 32)
	copy(derivedKey, secretKey)
	if len(secretKey) < 32 {
		h := sha1.Sum(secretKey)
		copy(derivedKey, h[:])
	} else if len(secretKey) > 32 {
		derivedKey = secretKey[:32]
	}

	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(password), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptPassword decrypts a base64 encoded, AES-GCM encrypted password string
func DecryptPassword(encryptedStr string, secretKey []byte) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encryptedStr)
	if err != nil {
		return "", err
	}
	derivedKey := make([]byte, 32)
	copy(derivedKey, secretKey)
	if len(secretKey) < 32 {
		h := sha1.Sum(secretKey)
		copy(derivedKey, h[:])
	} else if len(secretKey) > 32 {
		derivedKey = secretKey[:32]
	}

	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// WS-Discovery Models

const wsDiscoveryProbe = `<?xml version="1.0" encoding="utf-8"?>
<Envelope xmlns:tds="http://www.onvif.org/ver10/device/wsdl" xmlns="http://www.w3.org/2003/05/soap-envelope" xmlns:dn="http://www.onvif.org/ver10/network/wsdl" xmlns:wsa="http://schemas.xmlsoap.org/ws/2004/08/addressing">
  <Header>
    <wsa:MessageID>urn:uuid:%s</wsa:MessageID>
    <wsa:To>urn:schemas-xmlsoap-org:ws:2004:08:discovery</wsa:To>
    <wsa:Action>http://schemas.xmlsoap.org/ws/2004/08/discovery/Probe</wsa:Action>
  </Header>
  <Body>
    <Probe>
      <Types>dn:NetworkVideoTransmitter</Types>
      <Scopes />
    </Probe>
  </Body>
</Envelope>`

type ProbeMatchesEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		ProbeMatches struct {
			ProbeMatch []struct {
				EndpointReference struct {
					Address string `xml:"Address"`
				} `xml:"EndpointReference"`
				Types           string `xml:"Types"`
				Scopes          string `xml:"Scopes"`
				XAddrs          string `xml:"XAddrs"`
				MetadataVersion int    `xml:"MetadataVersion"`
			} `xml:"ProbeMatch"`
		} `xml:"ProbeMatches"`
	} `xml:"Body"`
}

type DiscoveredDevice struct {
	EndpointReference string
	XAddr             string
	Types             string
	Scopes            string
	IP                string
	Port              int
}

// SOAP Models

type SOAPEnvelope struct {
	XMLName xml.Name `xml:"s:Envelope"`
	S       string   `xml:"xmlns:s,attr"`
	Header  *SOAPHeader `xml:"s:Header,omitempty"`
	Body    SOAPBody `xml:"s:Body"`
}

type SOAPHeader struct {
	Security *SecurityHeader `xml:"wsse:Security,omitempty"`
}

type SecurityHeader struct {
	XMLName xml.Name `xml:"wsse:Security"`
	Wsse    string   `xml:"xmlns:wsse,attr"`
	Wsu     string   `xml:"xmlns:wsu,attr,omitempty"`
	MustUnderstand string `xml:"s:mustUnderstand,attr,omitempty"`
	UsernameToken *UsernameToken `xml:"wsse:UsernameToken,omitempty"`
}

type UsernameToken struct {
	Username string `xml:"wsse:Username"`
	Password *Password `xml:"wsse:Password,omitempty"`
	Nonce    *Nonce    `xml:"wsse:Nonce,omitempty"`
	Created  string `xml:"wsu:Created,omitempty"`
}

type Password struct {
	Type  string `xml:"Type,attr,omitempty"`
	Value string `xml:",chardata"`
}

type Nonce struct {
	EncodingType string `xml:"EncodingType,attr,omitempty"`
	Value        string `xml:",chardata"`
}

type SOAPBody struct {
	Content interface{} `xml:",any"`
}

// Requests

type GetDeviceInformation struct {
	XMLName xml.Name `xml:"tds:GetDeviceInformation"`
	Xmlns   string   `xml:"xmlns:tds,attr"`
}

type GetCapabilities struct {
	XMLName  xml.Name `xml:"tds:GetCapabilities"`
	Xmlns    string   `xml:"xmlns:tds,attr"`
	Category string   `xml:"tds:Category"`
}

type GetProfiles struct {
	XMLName xml.Name `xml:"trt:GetProfiles"`
	Xmlns   string   `xml:"xmlns:trt,attr"`
}

type StreamSetup struct {
	Stream    string `xml:"tt:Stream"`
	Transport struct {
		Protocol string `xml:"tt:Protocol"`
	} `xml:"tt:Transport"`
}

type GetStreamUri struct {
	XMLName      xml.Name    `xml:"trt:GetStreamUri"`
	XmlnsTrt     string      `xml:"xmlns:trt,attr"`
	XmlnsTt      string      `xml:"xmlns:tt,attr"`
	StreamSetup  StreamSetup `xml:"trt:StreamSetup"`
	ProfileToken string      `xml:"trt:ProfileToken"`
}

// SOAP Response Envelope

type SOAPResponseEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		GetDeviceInformationResponse struct {
			Manufacturer    string `xml:"Manufacturer"`
			Model           string `xml:"Model"`
			FirmwareVersion string `xml:"FirmwareVersion"`
			SerialNumber    string `xml:"SerialNumber"`
			HardwareId      string `xml:"HardwareId"`
		} `xml:"GetDeviceInformationResponse"`

		GetCapabilitiesResponse struct {
			Capabilities struct {
				Device struct {
					XAddr string `xml:"XAddr"`
				} `xml:"Device"`
				Media struct {
					XAddr string `xml:"XAddr"`
				} `xml:"Media"`
				PTZ struct {
					XAddr string `xml:"XAddr"`
				} `xml:"PTZ"`
			} `xml:"Capabilities"`
		} `xml:"GetCapabilitiesResponse"`

		GetProfilesResponse struct {
			Profiles []struct {
				Token string `xml:"token,attr"`
				Name  string `xml:"Name"`
			} `xml:"Profiles"`
		} `xml:"GetProfilesResponse"`

		GetStreamUriResponse struct {
			MediaUri struct {
				Uri string `xml:"Uri"`
			} `xml:"MediaUri"`
		} `xml:"GetStreamUriResponse"`

		Fault struct {
			Code struct {
				Value string `xml:"Value"`
			} `xml:"Code"`
			Reason struct {
				Text string `xml:"Text"`
			} `xml:"Reason"`
		} `xml:"Fault"`
	} `xml:"Body"`
}

// DiscoverONVIFDevices executes a UDP Multicast WS-Discovery query to detect local cameras
func DiscoverONVIFDevices(ctx context.Context, timeout time.Duration) ([]DiscoveredDevice, error) {
	multicastAddr := "239.255.255.250:3702"
	addr, err := net.ResolveUDPAddr("udp4", multicastAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve multicast address: %w", err)
	}

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("failed to listen on udp: %w", err)
	}
	defer conn.Close()

	// Generate random Message ID
	msgIDBytes := make([]byte, 16)
	_, _ = rand.Read(msgIDBytes)
	msgID := fmt.Sprintf("%x-%x-%x-%x-%x", msgIDBytes[0:4], msgIDBytes[4:6], msgIDBytes[6:8], msgIDBytes[8:10], msgIDBytes[10:])

	probePayload := fmt.Sprintf(wsDiscoveryProbe, msgID)

	_, err = conn.WriteTo([]byte(probePayload), addr)
	if err != nil {
		return nil, fmt.Errorf("failed to write to multicast: %w", err)
	}

	var devices []DiscoveredDevice
	seenEndpoints := make(map[string]bool)
	buf := make([]byte, 16384)

	err = conn.SetReadDeadline(time.Now().Add(timeout))
	if err != nil {
		return nil, err
	}

	for {
		select {
		case <-ctx.Done():
			return devices, ctx.Err()
		default:
		}

		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				break
			}
			return nil, fmt.Errorf("failed to read from udp: %w", err)
		}

		var envelope ProbeMatchesEnvelope
		if err := xml.Unmarshal(buf[:n], &envelope); err != nil {
			continue
		}

		for _, match := range envelope.Body.ProbeMatches.ProbeMatch {
			endpoint := match.EndpointReference.Address
			if endpoint == "" || seenEndpoints[endpoint] {
				continue
			}
			seenEndpoints[endpoint] = true

			xaddrs := strings.Fields(match.XAddrs)
			if len(xaddrs) == 0 {
				continue
			}

			ip, port := parseIPPortFromURL(xaddrs[0])

			devices = append(devices, DiscoveredDevice{
				EndpointReference: endpoint,
				XAddr:             xaddrs[0],
				Types:             match.Types,
				Scopes:            match.Scopes,
				IP:                ip,
				Port:              port,
			})
		}
	}

	return devices, nil
}

func parseIPPortFromURL(rawURL string) (string, int) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", 80
	}
	host := u.Host
	if strings.Contains(host, ":") {
		h, pStr, err := net.SplitHostPort(host)
		if err == nil {
			var port int
			fmt.Sscanf(pStr, "%d", &port)
			return h, port
		}
	}
	if u.Scheme == "https" {
		return host, 443
	}
	return host, 80
}

func createSOAPEnvelope(bodyContent interface{}, username, password string) (*SOAPEnvelope, error) {
	env := &SOAPEnvelope{
		S: "http://www.w3.org/2003/05/soap-envelope",
	}

	if username != "" {
		nonceBytes := make([]byte, 16)
		if _, err := rand.Read(nonceBytes); err != nil {
			return nil, err
		}
		nonceBase64 := base64.StdEncoding.EncodeToString(nonceBytes)
		created := time.Now().UTC().Format(time.RFC3339)

		hasher := sha1.New()
		hasher.Write(nonceBytes)
		hasher.Write([]byte(created))
		hasher.Write([]byte(password))
		digest := base64.StdEncoding.EncodeToString(hasher.Sum(nil))

		env.Header = &SOAPHeader{
			Security: &SecurityHeader{
				Wsse: "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd",
				Wsu:  "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd",
				MustUnderstand: "1",
				UsernameToken: &UsernameToken{
					Username: username,
					Password: &Password{
						Type:  "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest",
						Value: digest,
					},
					Nonce: &Nonce{
						EncodingType: "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary",
						Value:        nonceBase64,
					},
					Created: created,
				},
			},
		}
	}

	env.Body.Content = bodyContent
	return env, nil
}

// SendSOAPRequest sends a SOAP request with WS-Security header and returns the unmarshaled SOAP envelope response
func SendSOAPRequest(ctx context.Context, endpoint string, request interface{}, username, password string) (*SOAPResponseEnvelope, error) {
	env, err := createSOAPEnvelope(request, username, password)
	if err != nil {
		return nil, fmt.Errorf("failed to create soap envelope: %w", err)
	}

	payload, err := xml.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal soap envelope: %w", err)
	}

	xmlPayload := []byte(`<?xml version="1.0" encoding="utf-8"?>` + string(payload))

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(xmlPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send soap request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusInternalServerError {
		return nil, fmt.Errorf("http error: status %s, body: %s", resp.Status, string(bodyBytes))
	}

	var soapResp SOAPResponseEnvelope
	if err := xml.Unmarshal(bodyBytes, &soapResp); err != nil {
		return nil, fmt.Errorf("failed to decode soap response: %w (body: %s)", err, string(bodyBytes))
	}

	if soapResp.Body.Fault.Code.Value != "" {
		return &soapResp, fmt.Errorf("soap fault: code=%s, reason=%s",
			soapResp.Body.Fault.Code.Value, soapResp.Body.Fault.Reason.Text)
	}

	return &soapResp, nil
}

// PTZ SOAP Request Models

type Vector2D struct {
	X float64 `xml:"x,attr"`
	Y float64 `xml:"y,attr"`
}

type Vector1D struct {
	X float64 `xml:"x,attr"`
}

type PTZSpeed struct {
	PanTilt *Vector2D `xml:"tt:PanTilt,omitempty"`
	Zoom    *Vector1D `xml:"tt:Zoom,omitempty"`
}

type ContinuousMove struct {
	XMLName      xml.Name  `xml:"tptz:ContinuousMove"`
	XmlnsTptz    string    `xml:"xmlns:tptz,attr"`
	XmlnsTt      string    `xml:"xmlns:tt,attr"`
	ProfileToken string    `xml:"tptz:ProfileToken"`
	Velocity     PTZSpeed  `xml:"tptz:Velocity"`
	Timeout      string    `xml:"tptz:Timeout,omitempty"`
}
