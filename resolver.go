package main

import (
	"fmt"
	"log/slog"

	"github.com/miekg/dns"
)

var rootServers = []string{
	"198.41.0.4", // a.root-servers.net
	"199.9.14.201",
	"192.33.4.12",
}
var notFoundErr = fmt.Errorf("Couldn't find any answers for given query")

type Resolver struct{
	logger *slog.Logger
}

func (r Resolver) queryQ(q dns.Question, server string) *dns.Msg {
	msg := new(dns.Msg)
	c := new(dns.Client)
	msg.Question = append(msg.Question, q)
	resp, rtt, err := c.Exchange(msg, server+":53")
	r.logger.Debug(q.String(), "To: ", server, "RTT: ", rtt,)

	if err != nil {
		// retry with tcp
		slog.Warn("Got err during dns query request: ", "err", err)
		return nil
	}
	return resp
}

func (r Resolver) resolveQ(q dns.Question, depth int) ([]dns.RR, error) {
	if depth > 6{
		return nil, fmt.Errorf("Maximum resolving recursion depth reached")
	}
	servers := rootServers
	answer := make([]dns.RR, 0, 10)
	for range 20{
		for _, server := range servers{
			resp := r.queryQ(q, server)

			if len(resp.Answer) > 0 {
				answer = append(answer, resp.Answer...)
				if q.Qtype == dns.TypeA{
					var cnameResolved bool
					for _, rr := range resp.Answer{
						if rr.Header().Name == q.Name && rr.Header().Rrtype == dns.TypeA{
							cnameResolved = true
							break
						}
						if rr, ok := rr.(*dns.CNAME); ok{
							servers = rootServers
							q.Name = rr.Target
							r.logger.Debug("Resolving CNAME " + q.Name)
						}
					}
					if !cnameResolved {
						goto NextRound
					}
				}
				return answer, nil
			}

			// glue records
			// following glue records regardless
			var nextServers []string
			if len(resp.Extra) > 0{
				for _, extr := range resp.Extra{
					if rr, ok := extr.(*dns.A); ok{
						r.logger.Info("Got additional field " + rr.String())
						nextServers = append(nextServers, rr.A.String())
					}
				}
			}

			if len(nextServers) > 0{
				servers = nextServers
				goto NextRound 
			}

			if len(resp.Ns) > 0 {
				r.logger.Info("Got more than zero referals" )
				for _, ns := range resp.Ns {
					n, ok := ns.(*dns.NS)
					r.logger.Debug(fmt.Sprintf("%+v", n))
					if ok {
						nsq := dns.Question{Name: n.Ns, Qtype: dns.TypeA, Qclass: dns.ClassINET}
						// resolve NS; again starting with rootServers
						r.logger.Debug("RESOLVING NS ")
						rr, err := r.resolveQ(nsq, depth+1)
						if err != nil{
							r.logger.Warn("Err during resolve referals: ", "referal name", n.Hdr.Name, "err", err)
							continue
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
		return nil, notFoundErr 

		NextRound:
	}
	return answer, notFoundErr
}

func handleAll(w dns.ResponseWriter, m *dns.Msg) {
	msg := new(dns.Msg)
	msg.SetReply(m)

	for _, q := range m.Question {
		rr, err  := Resolver{slog.Default()}.resolveQ(q, 0)		
		if err != nil{
			// write err as dns err
			slog.Error("Got err during resolve: ", "err", err)
			return
		}


		if len(rr) > 0 {
			for _, r := range rr {
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
	// fmt.Println("sent ", len(msg.Answer))
	// fmt.Println(msg.Truncated)
	// fmt.Println(msg.Compress)

	if err := w.WriteMsg(msg); err != nil {
		slog.Error("WriteMsg failed: ", "err: ", err)
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
			slog.Error(err.Error())
		}
		wg <- struct{}{}
	}()

	tcpServer := dns.Server{Addr: "127.0.0.1:5356", Net: "tcp"}

	go func(){
		err := tcpServer.ListenAndServe()

		if err != nil {
			slog.Error(err.Error())
		}

		wg <- struct{}{}
	}()
	slog.Info("Started tcp and udp servers")
	<-wg
}
