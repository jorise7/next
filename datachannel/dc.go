package datachannel

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/chzyer/flow"
	"github.com/chzyer/next/packet"
	"gopkg.in/logex.v1"
)

type DC struct {
	cfg       *Config
	flow      *flow.Flow
	session   *packet.SessionIV
	conn      net.Conn
	writeChan chan *packet.Packet
	exitError error

	heartBeat *packet.HeartBeatStage
}

type Config struct {
	OnClose func()
}

func New(f *flow.Flow, conn net.Conn, session *packet.SessionIV, cfg *Config) *DC {

	dc := &DC{
		session:   session,
		conn:      conn,
		cfg:       cfg,
		writeChan: make(chan *packet.Packet, 4),
	}

	f.ForkTo(&dc.flow, dc.Close)
	dc.heartBeat = packet.NewHeartBeatStage(
		dc.flow, 3*time.Second, dc.Name(), func(err error) {
			dc.exitError = fmt.Errorf("monitor: %v", err)
			dc.Close()
		})
	return dc
}

func (d *DC) Run(in <-chan *packet.Packet, out chan<- *packet.Packet) {
	go d.writeLoop(in)
	go d.readLoop(out)
}

func (d *DC) readLoop(out chan<- *packet.Packet) {
	d.flow.Add(1)
	defer d.flow.DoneAndClose()

	buf := bufio.NewReader(d.conn)
loop:
	for {
		p, err := packet.Read(d.session, buf)
		if err != nil {
			if !strings.Contains(err.Error(), "closed") {
				d.exitError = fmt.Errorf("read error: %v", err)
			}
			break
		}
		switch p.Type {
		case packet.HEARTBEAT:
			d.writeChan <- p.Reply(heart)
		case packet.HEARTBEAT_R:
			d.heartBeat.Receive(p.IV)
		default:
			select {
			case <-d.flow.IsClose():
				break loop
			case out <- p:
			}
		}
	}
}

var heart = []byte(nil)

func (d *DC) write(p *packet.Packet) error {
	_, err := d.conn.Write(p.Marshal(d.session))
	return err
}

func (d *DC) writeLoop(in <-chan *packet.Packet) {
	d.flow.Add(1)
	defer d.flow.DoneAndClose()
	heartBeatTicker := time.NewTicker(time.Second)
	defer heartBeatTicker.Stop()

	var err error
loop:
	for {
		select {
		case <-d.flow.IsClose():
			break loop
		case <-heartBeatTicker.C:
			p := d.heartBeat.New()
			p.Payload = heart
			err = d.write(p)
			d.heartBeat.Add(p.IV)
		case p := <-d.writeChan:
			err = d.write(p)
		case p := <-in:
			err = d.write(p)
		}
		if err != nil {
			if !strings.Contains(err.Error(), "closed") {
				d.exitError = fmt.Errorf("write error: %v", err)
			}
			break
		}
	}
}

func (d *DC) GetStat() *packet.HeartBeatStat {
	return d.heartBeat.GetStat()
}

func (d *DC) Name() string {
	return fmt.Sprintf("[%v -> %v]",
		d.conn.LocalAddr(),
		d.conn.RemoteAddr(),
	)
}

func (d *DC) Close() {
	if d.exitError != nil {
		logex.Info(d.Name(), "closed by:", d.exitError)
	}
	d.conn.Close()
	d.flow.Close()
	if d.cfg.OnClose != nil {
		d.cfg.OnClose()
	}
}

func (d *DC) GetUserId() int {
	return int(d.session.UserId)
}

func (d *DC) GetSession() *packet.SessionIV {
	return d.session
}

func DialDC(host string, f *flow.Flow, session *packet.SessionIV,
	onClose func(), in, out chan *packet.Packet) (*DC, error) {

	conn, err := net.DialTimeout("tcp", host, time.Second)
	if err != nil {
		return nil, logex.Trace(err)
	}
	if err := ClientCheckAuth(conn, session); err != nil {
		return nil, logex.Trace(err)
	}
	dc := New(f, conn, session, &Config{
		OnClose: onClose,
	})
	dc.Run(in, out)
	return dc, nil
}
