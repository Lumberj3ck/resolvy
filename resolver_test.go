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


func TestNsResolvePath(t *testing.T){
	q := NewQuestion("dns1.p01.nsone.net", dns.TypeA)
	r := Resolver{slog.New(slog.NewTextHandler(io.Discard, nil))}
	answ_rr, err := r.resolveQ(q, 0)

	if err != nil{
		t.Error("err during dns exchange: ", err.Error())
	}
	if len(answ_rr) == 0{
		t.Error("No domain name found")
	}
}

func TestCNAMEResolvePath(t *testing.T){
	testCases := []string{"blog.dnsimple.com", "www.github.com", "www.apple.com"}

	for _, test := range testCases{
		t.Run(test, func (t *testing.T){
			q := NewQuestion(test, dns.TypeA)
			r := Resolver{slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))}
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


