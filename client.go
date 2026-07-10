package main

import (
	"log"

	"github.com/miekg/dns"
)

func main() {
	m := new(dns.Msg)
	serverAddr := "127.0.0.1:5356"
	name := "google.com"
	// m.SetQuestion(dns.Fqdn(name), dns.TypeA)
	// m.RecursionDesired = false
	//
	c := new(dns.Client)
	c.Net = "udp"
	// d, _, err := c.Exchange(m, serverAddr)
	// if err != nil{
	// 	log.Fatalf("err during dns exchange: ", err.Error())
	// }
	// log.Println(d)
	// log.Println(d.Answer)

	log.Println("-----------------------------")
	m = new(dns.Msg)
	name = "www.youtube.com"
	m.SetQuestion(dns.Fqdn(name), dns.TypeA)
	m.RecursionDesired = false

	d, _, err := c.Exchange(m, serverAddr)
	if err != nil{
		log.Fatalf("err during dns exchange: ", err.Error())
	}

	log.Println(d)
	log.Println(d.Answer, len(d.Answer))
	log.Println("Is Truncated: ", d.Truncated)
	if d.Truncated{
		c.Net = "tcp"
		d, _, err := c.Exchange(m, serverAddr)

		if err != nil{
			log.Fatalf("err during dns exchange: ", err.Error())
		}

		log.Println(d)
		log.Println(d.Answer, len(d.Answer))
	}
}
