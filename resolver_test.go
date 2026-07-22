package main

import (
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/miekg/dns"
)

func NewMsg(name string, qtype dns.Type) *dns.Msg{
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), uint16(qtype))
	return m
}

func NewQuestion(name string, qtype uint16) dns.Question{
	return dns.Question{Name: dns.Fqdn(name), Qtype: qtype, Qclass: dns.ClassINET}
}

func NewClient() *dns.Client{
	c := new(dns.Client)
	c.Net = "udp"
	return c
}

func TestCacheMatching(t *testing.T){
	testCases := []struct{test string; want string}{{"blog.dnsimple.com",".com"}, {"www.github.com", "www.github.com"}, {"www.apple.om", "apple.om"}, {"dns1.01.none.net", ".net"}}

	for _, Tcase := range testCases{
		c := make(Cache)
		// NS_RR.Hdr.Name -- is actuall ownership of this refference
		wantRR := NS_RR{ NS: dns.NS{ Hdr: dns.RR_Header{ Name:   Tcase.want, Rrtype: dns.TypeNS, Class:  dns.ClassINET, }, Ns: Tcase.want}, }

		c[Tcase.want] = wantRR
		t.Log("PUSH", Tcase.test)

		t.Run(Tcase.test, func(t *testing.T) {
			resp := c.getClosestZone(Tcase.test)
			t.Log(resp)
			if len(resp) == 0 || resp[0].Hdr != wantRR.Hdr{
				t.Fail()
			}
		})
	}
}

func TestCNAMEResolvePath(t *testing.T){
	testCases := []string{"blog.dnsimple.com", "www.github.com", "www.apple.com", "dns1.p01.nsone.net"}
	v := os.Getenv("RESOLVY_LOGS")
	var writer io.Writer
	if v == "" {
		writer = io.Discard
	} else {
		writer = os.Stdout
	}
	logger := slog.New(slog.NewTextHandler(writer, &slog.HandlerOptions{Level: slog.LevelDebug}))

	for _, test := range testCases{
		t.Run(test, func (t *testing.T){
			q := NewQuestion(test, dns.TypeA)

			r := Resolver{logger: logger, Cache: make(Cache)}
			answ_rr, err := r.resolveQ(q, 0)

			if err != nil{
				t.Error("err during dns exchange: ", err.Error())
			}
			if len(answ_rr) == 0{
				t.Error("No domain name found")
			}
			for _, rr  := range answ_rr{
				t.Log(rr.String())
			}
		})
	}
}


