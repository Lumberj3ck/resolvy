package main

import (
	"fmt"
	"log"

	"github.com/miekg/dns"
)

var rootServers = []string{
	"198.41.0.4", // a.root-servers.net
	"199.9.14.201",
	"192.33.4.12",
}

func query(server, name string, qtype uint16) (*dns.Msg, error) {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)
	m.RecursionDesired = false

	c := new(dns.Client)
	d, _, err := c.Exchange(m, server+":53")
	return d, err
}

func resolve(name string, qtype uint16) ([]dns.RR, error) {
	servers := rootServers

	for depth := 0; depth < 20; depth++ {
		for _, server := range servers {
			resp, err := query(server, name, qtype)
			if err != nil {
				continue
			}

			if len(resp.Answer) > 0 {
				return resp.Answer, nil
			}

			// Follow glue records in Additional section.
			var nextServers []string
			for _, rr := range resp.Extra {
				if a, ok := rr.(*dns.A); ok {
					nextServers = append(nextServers, a.A.String())
				}
			}

			if len(nextServers) > 0 {
				servers = nextServers
				goto nextRound
			}

			// If no glue, resolve NS names separately.
			for _, rr := range resp.Ns {
				if ns, ok := rr.(*dns.NS); ok {
					nsAnswers, err := resolve(ns.Ns, dns.TypeA)
					if err != nil {
						continue
					}
					for _, ans := range nsAnswers {
						if a, ok := ans.(*dns.A); ok {
							nextServers = append(nextServers, a.A.String())
						}
					}
				}
			}

			if len(nextServers) > 0 {
				servers = nextServers
				goto nextRound
			}
		}

		return nil, fmt.Errorf("resolution failed for %s", name)

	nextRound:
	}

	return nil, fmt.Errorf("too many referrals")
}

func queryQ(q dns.Question, server string) *dns.Msg {
	msg := new(dns.Msg)
	c := new(dns.Client)
	msg.Question = append(msg.Question, q)
	resp, rtt, err := c.Exchange(msg, server+":53")
	log.Println("RTT: ", rtt)
	// log.Println("RESP: ", resp)

	if err != nil {
		// retry with tcp
		log.Println(err)
		return nil
	}
	return resp
}

func resolveQ(q dns.Question, depth int) []dns.RR {
	if depth > 6{
		return nil
	}
	servers := rootServers
	for range 20{
		for _, server := range servers{
			log.Println("resolving: ", q.Name, q.Qtype)
			resp := queryQ(q, server)

			if len(resp.Answer) > 0 {
				return resp.Answer
			}

			// glue records
			// following glue records regardless
			var nextServers []string
			if len(resp.Extra) > 0{			
				for _, extr := range resp.Extra{
					if rr, ok := extr.(*dns.A); ok{
						nextServers = append(nextServers, rr.A.String())
					}
				}
			}

			if len(nextServers) > 0{
				servers = nextServers
				goto NextRound 
			}

			if len(resp.Ns) > 0 {
				for _, ns := range resp.Ns {
					n, ok := ns.(*dns.NS)
					if ok {
						nsq := dns.Question{Name: n.Ns, Qtype: dns.TypeA, Qclass: dns.ClassINET}
						// resolve NS; again starting with rootServers
						log.Println("RESOLVING NS ")
						rr := resolveQ(nsq, depth+1)
						if rr == nil {
							return  nil
						}
						for _, r := range rr{
							if ar, ok := r.(*dns.A); ok{
								nextServers = append(nextServers, ar.A.String())
							}
						}
					}
				}
			}

			if len(nextServers) > 0{
				servers = nextServers
				goto NextRound 
			}
		}
		return nil

		NextRound:
	}
	return []dns.RR{}
}

func handleAll(w dns.ResponseWriter, m *dns.Msg) {
	msg := new(dns.Msg)
	msg.SetReply(m)

	for _, q := range m.Question {
		fmt.Println(q.Name, "Type: ", q.Qtype)
		rr := resolveQ(q, 0)

		if len(rr) > 0 {
			for _, r := range rr {
				msg.Answer = append(msg.Answer, r)
				msg.Answer = append(msg.Answer, r)
				msg.Answer = append(msg.Answer, r)
				msg.Answer = append(msg.Answer, r)
			}
		}
	}

	if w.RemoteAddr().Network() == "udp"{
		size := dns.MinMsgSize

		if opt := m.IsEdns0(); opt != nil{
			size = int(opt.UDPSize())
		}

		msg.Truncate(size)
	}
	fmt.Println("sent ", len(msg.Answer))
	fmt.Println(msg.Truncated)
	fmt.Println(msg.Compress)

	if err := w.WriteMsg(msg); err != nil {
		log.Println("WriteMsg failed:", err)
	}
}

func main() {
	// name := "www.example.com"
	// answers, err := resolve(name, dns.TypeAAAA)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	//
	// for _, rr := range answers {
	// 	fmt.Println(strings.TrimSpace(rr.String()))
	// }
	udpServer := dns.Server{Addr: "127.0.0.1:5356", Net: "udp"}
	dns.HandleFunc(".", handleAll)
	var wg chan struct{}

	go func(){
		err := udpServer.ListenAndServe()

		if err != nil {
			log.Fatal(err)
		}
		wg <- struct{}{}
	}()

	tcpServer := dns.Server{Addr: "127.0.0.1:5356", Net: "tcp"}

	go func(){
		err := tcpServer.ListenAndServe()

		if err != nil {
			log.Fatal(err)
		}

		wg <- struct{}{}
	}()
	log.Println("Started tcp and udp servers")
	<-wg
}
