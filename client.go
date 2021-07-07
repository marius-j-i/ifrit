package ifrit

import (
	"crypto/x509/pkix"
	"errors"
	"fmt"

	log "github.com/inconshreveable/log15"

	"github.com/joonnna/ifrit/comm"
	"github.com/joonnna/ifrit/core"
	"github.com/joonnna/ifrit/netutil"
	"github.com/spf13/viper"
)

type Client struct {
	node *core.Node
}

type ClientConfig struct {
	UdpPort, TcpPort   int
	Hostname, CertPath string
}

var (
	errNoData      = errors.New("Supplied data is of length 0")
	errNoCaAddress = errors.New("Config does not contain address of CA")
	errNoClientArg = errors.New("Client argument zero")
)

/* Creates and returns a new ifrit client instance.
 *
 * Change: Added argument struct containing specifiable context for ifrit-client. - marius
 */
func NewClient(cliCfg *ClientConfig) (*Client, error) {
	var cu *comm.CryptoUnit

	if cliCfg == nil {
		return nil, errNoClientArg
	}

	err := readConfig()
	if err != nil {
		return nil, err
	}

	udpConn, udpAddr, err := netutil.ListenUdp(cliCfg.Hostname, cliCfg.UdpPort)
	if err != nil {
		return nil, err
	}

	l, err := netutil.GetListener(cliCfg.Hostname, cliCfg.TcpPort)
	if err != nil {
		return nil, err
	}

	log.Debug("addrs", "rpc", l.Addr().String(), "udp", udpAddr)

	pk := pkix.Name{
		/* Tell crypto-unit where this client can be reached. */
		Locality: []string{fmt.Sprintf("%s:%d", cliCfg.Hostname, cliCfg.TcpPort), udpAddr},
	}

	caAddr := viper.GetString("ca_addr")

	if cliCfg.CertPath == "" {
		cu, err = comm.NewCu(pk, caAddr, cliCfg.Hostname)
		if err != nil {
			return nil, err
		}
	} else {
		cu, err = comm.LoadCu(cliCfg.CertPath, pk, caAddr)
		if err != nil {
			return nil, err
		}
	}

	c, err := comm.NewComm(cu.Certificate(), cu.CaCertificate(), cu.Priv(), l)
	if err != nil {
		return nil, err
	}

	udpServer, err := comm.NewUdpServer(cu, udpConn)
	if err != nil {
		return nil, err
	}

	n, err := core.NewNode(c, udpServer, cu, cu)
	if err != nil {
		return nil, err
	}

	return &Client{
		node: n,
	}, nil
}

// Client starts operating.
func (c *Client) Start() {
	c.node.Start()
}

// Stops client operations.
// The client cannot be used after callling Close.
func (c *Client) Stop() {
	c.node.Stop()
}

// Returns the address (ip:port, rpc endpoint) of all other ifrit clients in the network which is currently believed to be alive.
func (c *Client) Members() []string {
	return c.node.LiveMembers()
}

// Returns ifrit's internal ID generated by the trusted CA
func (c *Client) Id() string {
	return c.node.Id()
}

// Returns the address(ip:port) of the ifrit client.
// Can be directly used as entry addresses in the config.
func (c *Client) Addr() string {
	return c.node.Addr()
}

// Signs the provided content with the internal private key of ifrit.
func (c *Client) Sign(content []byte) ([]byte, []byte, error) {
	return c.node.Sign(content)
}

// Checks if the given content is correctly signed by the public key
// belonging to the given node id.
// The id represents another node in the Fireflies network, if the id
// is not recongnized false is returned.
func (c *Client) VerifySignature(r, s, content []byte, id string) bool {
	return c.node.Verify(r, s, content, id)
}

// Sends the given data to the given destination.
// The caller must ensure that the given data is not modified after calling this function.
// The returned channel will be populated with the response.
// If the destination could not be reached or timeout occurs, nil will be sent through the channel.
// The response data can be safely modified after receiving it.
func (c *Client) SendTo(dest string, data []byte) chan []byte {
	ch := make(chan []byte, 1)

	go c.node.SendMessage(dest, ch, data)

	return ch
}

// Same as SendTo, but destination is now the Ifrit id of the receiver.
// Returns an error if no observed peer has the specified  destination id.
func (c *Client) SendToId(destId []byte, data []byte) (chan []byte, error) {
	addr, err := c.node.IdToAddr(destId)
	if err != nil {
		return nil, err
	}

	ch := make(chan []byte, 1)

	go c.node.SendMessage(addr, ch, data)

	return ch, err
}

// Returns a pair of channels used for bi-directional streams, given the destination. The first channel
// is the input stream to the server and the second stream is the reply stream from the server.
// To close the stream, close the input channel. The reply stream is open as long as the server sends messages
// back to the client. The caller must ensure that the reply stream does not block by draining the buffer so that the stream session can complete.
// Note: it is adviced to implement an aknowledgement mechanism to avoid an untimely closing of a channel and loss of messages.
func (c *Client) OpenStream(dest string) (chan []byte, chan []byte) {
	inputStream := make(chan []byte)
	replyStream := make(chan []byte)

	go c.node.OpenStream(dest, inputStream, replyStream)

	return inputStream, replyStream
}

// Registers the given function as the stream handler.
// Invoked when the client opens a stream. The callback accepts two channels -
// an unbuffered input channel and an unbuffered channel used for replying to the client.
// The caller must close the reply channel to signal that the stream is closing.
// See the note in OpenStream().
func (c *Client) RegisterStreamHandler(streamHandler func(chan []byte, chan []byte)) {
	c.node.SetStreamHandler(streamHandler)
}

// Registers the given function as the message handler.
// Invoked each time the ifrit client receives an application message (another client sent it through SendTo), this callback will be invoked.
// The returned byte slice will be sent back as the response.
// If error is non-nil, it will be returned as the response.
// All responses will be received on the sending side through a channel,
// see SendTo documentation for details.
func (c *Client) RegisterMsgHandler(msgHandler func([]byte) ([]byte, error)) {
	c.node.SetMsgHandler(msgHandler)
}

// Registers the given function as the gossip handler.
// Invoked each time ifrit receives application gossip.
// The returned byte slice will be sent back as the response.
// If the callback returns a non-nil error, it will be sent back as the response instead.
func (c *Client) RegisterGossipHandler(gossipHandler func([]byte) ([]byte, error)) {
	c.node.SetGossipHandler(gossipHandler)
}

// Registers the given function as the gossip response handler.
// Invoked when ifrit receives a response after gossiping application data.
// All responses originates from a gossip handler invocation.
// If the ResponseHandler is not registered or nil, responses will be discarded.
func (c *Client) RegisterResponseHandler(responseHandler func([]byte)) {
	c.node.SetResponseHandler(responseHandler)
}

// Replaces the gossip set with the given data.
// This data will be exchanged with neighbors in each gossip interaction.
// Recipients will receive it through the message handler callback.
// The response generated by the message handler callback will be sent back and invoke the response handler callback.
func (c *Client) SetGossipContent(data []byte) error {
	if len(data) <= 0 {
		return errNoData
	}

	c.node.SetExternalGossipContent(data)

	return nil
}

func (c *Client) SavePrivateKey(path string) error {
	return c.node.SavePrivateKey(path)
}

func (c *Client) SaveCertificate(path string) error {
	return c.node.SaveCertificate(path)
}

func readConfig() error {
	viper.SetConfigName("ifrit_config")
	viper.AddConfigPath("/var/tmp")
	viper.AddConfigPath(".")

	viper.SetConfigType("yaml")

	err := viper.ReadInConfig()
	if err != nil {
		return err
	}

	// Behavior variables
	viper.SetDefault("gossip_interval", 10)
	viper.SetDefault("monitor_interval", 10)
	viper.SetDefault("view_update_interval", 10)
	viper.SetDefault("ping_limit", 3)
	viper.SetDefault("pings_per_interval", 3)
	viper.SetDefault("removal_timeout", 60)
	viper.SetDefault("max_concurrent_messages", 5)
	viper.SetDefault("use_compression", true)

	// Visualizer specific
	viper.SetDefault("viz_update_interval", 10)

	viper.SafeWriteConfig()

	return nil
}
