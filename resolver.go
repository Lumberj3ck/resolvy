package main

import (
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/miekg/dns"
)

var safeBelt = []NS_RR{
	{
		ip: net.ParseIP("198.41.0.4"),
		NS: dns.NS{
			Hdr: dns.RR_Header{
				Name:   ".",
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
			},
			Ns: "a.root-servers.net.",
		},
	},
	{
		ip: net.ParseIP("199.9.14.201"),
		NS: dns.NS{
			Hdr: dns.RR_Header{
				Name:   ".",
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
			},
			Ns: "b.root-servers.net.",
		},
	},
	{
		ip: net.ParseIP("192.33.4.12"),
		NS: dns.NS{
			Hdr: dns.RR_Header{
				Name:   ".",
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
			},
			Ns: "c.root-servers.net.",
		},
	},
}

var notFoundErr = fmt.Errorf("Couldn't find any answers for given query")

type Resolver struct {
	logger *slog.Logger
	Cache  Cache
	mu     sync.Mutex
}

func (r *Resolver) queryQ(q dns.Question, server string) *dns.Msg {
	msg := new(dns.Msg)
	c := new(dns.Client)
	msg.Question = append(msg.Question, q)
	resp, rtt, err := c.Exchange(msg, server+":53")
	r.logger.Debug(q.String(), "To: ", server, "RTT: ", rtt)

	if err != nil {
		// retry with tcp
		slog.Warn("Got err during dns query request: ", "err", err)
		return nil
	}
	return resp
}

// NS_RR.Hdr.Name -- is actuall ownership of this refference
type NS_RR struct {
	ip net.IP
	dns.NS
}

type Cache map[string][]NS_RR

func (c Cache) getClosestZone(name string, match int) string {
	// www.apple.com
	var clossestZone string

	currMatch := match
	for zone, _ := range c {
		m := dns.CompareDomainName(dns.CanonicalName(zone), dns.CanonicalName(name))
		if dns.IsSubDomain(dns.CanonicalName(zone), dns.CanonicalName(name)) && m >= currMatch {
			clossestZone = zone
			currMatch = m
		}
	}

	if len(clossestZone) == 0 {
		clossestZone = "."
	}
	return clossestZone
}

func (r *Resolver) resolveQ(q dns.Question, depth int) ([]dns.RR, error) {
	// loop
	// s := get_the_closest_server(q.Name)
	// s - is_available? -> no -> resolve in gorutine
	//   |
	//  ask s about q.Name
	//        |
	//      response
	//        |
	//    do we have answer?  -> yes, return, if CNAME and qtype A change SNAME -- to CNAME
	//     /     \
	//    /       \
	//  ns ref     glue
	//  cache       find cache and add type.A IP
	//
	//  what to use for cache
	//  map[responsible_zone]dns.A
	// a structure which describes the name servers and the
	//             zone which the resolver is currently trying to query.
	//             This structure keeps track of the resolver's current
	//             best guess about which name servers hold the desired
	//             information; it is updated when arriving information
	//             changes the guess.  This structure includes the
	//             equivalent of a zone name, the known name servers for
	//             the zone, the known addresses for the name servers, and
	//             history information which can be used to suggest which
	//             server is likely to be the best one to try next.  The
	//             zone name equivalent is a match count of the number of
	//             labels from the root down which SNAME has in common with
	//             the zone being queried; this is used as a measure of how
	//             "close" the resolver is to SNAME

	// var visited map[string]bool
	var match int
	for range 20 {
		zone := r.Cache.getClosestZone(q.Name, match)

		servers := r.Cache[zone]

		var serverIP net.IP
		resolving := make(map[string]bool)
		for len(serverIP.String()) == 0 {
			for i := range servers {
				server := servers[i]

				if len(server.ip.String()) == 0 {
					if resolving[server.Ns] {
						continue
					}
					go func() {
						q := dns.Question{Name: server.Ns, Qtype: dns.TypeA, Qclass: dns.ClassINET}
						resp, err := r.resolveQ(q, depth+1)
						if err != nil {
							return
						}

						r.mu.Lock()
						for _, rr := range resp {
							if rr, ok := rr.(*dns.A); ok {
								servers[i].ip = rr.A
							}
						}
						delete(resolving, server.Ns)
						r.mu.Lock()
					}()
				} else {
					serverIP = server.ip
					break
				}
			}
		}
	}

	return nil, nil
}

func handleAll(w dns.ResponseWriter, m *dns.Msg) {
	msg := new(dns.Msg)
	msg.SetReply(m)
	resolver := Resolver{slog.Default(), make(Cache), sync.Mutex{}}

	for _, q := range m.Question {
		rr, err := resolver.resolveQ(q, 0)
		if err != nil {
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

	if w.RemoteAddr().Network() == "udp" {
		size := dns.MinMsgSize

		if opt := m.IsEdns0(); opt != nil {
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

	go func() {
		err := udpServer.ListenAndServe()

		if err != nil {
			slog.Error(err.Error())
		}
		wg <- struct{}{}
	}()

	tcpServer := dns.Server{Addr: "127.0.0.1:5356", Net: "tcp"}

	go func() {
		err := tcpServer.ListenAndServe()

		if err != nil {
			slog.Error(err.Error())
		}

		wg <- struct{}{}
	}()
	slog.Info("Started tcp and udp servers")
	<-wg
}
