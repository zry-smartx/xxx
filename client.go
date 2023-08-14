package main

import (
	"fmt"
	"io"
	"net"
	"sync"
	"strconv"
	"strings"
	"time"
	"math/big"
	"crypto/rand"
)




// listener
var (
	myIp         = "192.168.109.128"
	myListenPort = 10005
	mySendPort   = 10006
	allIPs       = []string{"192.168.109.128", "192.168.109.129", "192.168.109.130"}
)

func findindex(allips []string, myip string) int {
	for i, ip := range allips {
		if ip == myip {
			return i
		}
	}
	return -1
}

type MessageListener struct {
	Ip        string
	Port      int
	mapLock   sync.RWMutex
	sender    *MessageSender
}

func NewMessageListener(ip string, port int, ms *MessageSender) *MessageListener {
	messagelistener := &MessageListener{
		Ip:        ip,
		Port:      port,
		sender:    ms,
	}
	return messagelistener
}

func (this *MessageListener) Start() {
	// openlistener
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", this.Ip, this.Port))
	if err != nil {
		fmt.Println("net.Listen err:", err)
		return
	}
	fmt.Println("[log]local listener started")
	defer listener.Close()
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("listener.Accept err:", err)
			continue
		}
		go this.Handler(conn)
	}
}

func (this *MessageListener) Handler(conn net.Conn) {
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if n == 0 {
				return
			}
			if err != nil && err != io.EOF {
				fmt.Println("Conn Read err", err)
				return
			}
			msg := string(buf[:n-1])
			// process data from other node
			if msg == "all nodes online" {
				fmt.Println("[log]All Nodes Online!")
				for _, ip := range allIPs {
					_, ok := this.sender.Conns[ip]
					if myIp != ip && !ok {
						conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", ip, myListenPort))
						if err != nil {
							continue
						}
						this.sender.Conns[ip] = conn
					}
				}
				this.sender.step1_generate()
			} else if strings.Contains(msg, "[Cik Broadcast from") {
				fmt.Println("[cik broadcast]")
				ip := msg[20:35]
				values := msg[36:]
				this.sender.CheckBroadCast(ip, values)
			} else if strings.Contains(msg, "[sij,sij' Private from") {
				fmt.Println("[sij,sij' private]")
				ip := msg[23:38]
				values := msg[39:]
				this.sender.CheckBroadCast(ip, values)
			} else {
				fmt.Println(msg)
			}
		}
	}()
}





// sender
type MessageSender struct {
	Ip             string
	Port           int
	Conns          map[string]net.Conn
	onlinenum      int
	pedersenvss    *PedersenVSS
}

func NewMessageSender(myIp string, myPort int) *MessageSender {
	client := &MessageSender{
		Ip:    myIp,
		Port:  myPort,
		Conns: make(map[string]net.Conn),
	}
	p := NewPedersenVSS()
	client.pedersenvss = p
	for _, ip := range allIPs {
		if myIp != ip {
			conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", ip, myListenPort))
			if err != nil {
				//fmt.Println("[log]Node", ip, "not online yet")
				continue
			}
			client.Conns[ip] = conn
			//fmt.Println("[log]Node", ip, "online!")
			//fmt.Println("[log]Establish Connection with", ip)
		}
	}
	client.onlinenum = len(client.Conns)
	return client
}

func (this *MessageSender) step1_generate() {
	time.Sleep(2 * time.Second)
	fmt.Println("[log]========== Step1 Pedersen-VSS Generate and Verify ==========")
	time.Sleep(3 * time.Second)

	// broadcast modexp elements
	send := "[Cik Broadcast from " + myIp + "]"
	for i:=0; i<this.pedersenvss.t+1; i++ {
		C_ik := big.NewInt(1)
		C_ik.Mul(C_ik, ModExp(this.pedersenvss.g, this.pedersenvss.Polynomial1[i] , this.pedersenvss.N))
		C_ik.Mul(C_ik, ModExp(this.pedersenvss.h, this.pedersenvss.Polynomial2[i] , this.pedersenvss.N))
		C_ik.Mod(C_ik, this.pedersenvss.N)
		send += (C_ik.String()+" ")
	}
	for _, value := range this.Conns {
		_, err := value.Write([]byte(send[:len(send)-1]+"\n"))
		if err != nil {
			fmt.Println(err)
		}
	}

	// secretly send two polynomials
	time.Sleep(5 * time.Second)
	f1 := make([]*big.Int, 0)
	f2 := make([]*big.Int, 0)
	bigj := new(big.Int)
	for j:=1; j<=len(allIPs); j++ {
		bigj, _ = bigj.SetString(strconv.Itoa(j), 10)
		result := new(big.Int)
		power, _ := new(big.Int).SetString("1", 10)
		for _, ele := range this.pedersenvss.Polynomial1 {
			tmp := new(big.Int).Mul(ele, new(big.Int).Set(power))
			result.Add(result, tmp)
			power.Mul(power, bigj)
		}
		f1 = append(f1, result)
		result = big.NewInt(1)
		for _, ele := range this.pedersenvss.Polynomial2 {
			tmp := new(big.Int).Mul(ele, new(big.Int).Set(power))
			result.Add(result, tmp)
			power.Mul(power, bigj)
		}
		f2 = append(f2, result)	
	}
	for ip, value := range this.Conns {
		if ip != myIp {
			index := findindex(allIPs, ip)
			_, err := value.Write([]byte("[sij,sij' Private from " + myIp + "]" + f1[index].String() + " " + f2[index].String() + "\n"))
			if err != nil {
				fmt.Println(err)
			}
		}
	}
}

func (this *MessageSender) CheckBroadCast(ip string, values string) {
	fmt.Println(ip)
	fmt.Println(values)
}





// PedersenVSS
type PedersenVSS struct {
	p1            *big.Int
	p2            *big.Int
	p3            *big.Int
	t             int
	N             *big.Int
	Polynomial1   []*big.Int
	Polynomial2   []*big.Int
	z             *big.Int
	g             *big.Int
	h             *big.Int
}

func NewPedersenVSS() *PedersenVSS {
	a, _ := new(big.Int).SetString("1363895147340162124487750544377566700025348452567", 10)
	b, _ := new(big.Int).SetString("1257354545315887944833595666025792933231792977521", 10)
	c, _ := new(big.Int).SetString("1296657106138026641358592699056954007605324218609", 10)
	
	pedersenvss := &PedersenVSS{
		p1:          a,
		p2:          b,
		p3:          c,
		t:           len(allIPs)/2,
		Polynomial1: make([]*big.Int, 0),
		Polynomial2: make([]*big.Int, 0),
		g:           big.NewInt(3),
		h:           big.NewInt(65537),
	}

	pedersenvss.N = new(big.Int)
	pedersenvss.N.Mul(pedersenvss.p1, pedersenvss.p2)
	pedersenvss.N.Mul(pedersenvss.N, pedersenvss.p3)

	for i := 0; i < pedersenvss.t+1; i++ {
		randomBigInt, err := rand.Int(rand.Reader, pedersenvss.N)
		if err != nil {
			fmt.Println("Error generating random number:", err)
			return nil
		}
		pedersenvss.Polynomial1 = append(pedersenvss.Polynomial1, randomBigInt)
	}
	for i := 0; i < pedersenvss.t+1; i++ {
		randomBigInt, err := rand.Int(rand.Reader, pedersenvss.N)
		if err != nil {
			fmt.Println("Error generating random number:", err)
			return nil
		}
		pedersenvss.Polynomial2 = append(pedersenvss.Polynomial2, randomBigInt)
	}
	pedersenvss.z = pedersenvss.Polynomial1[0]
	
	return pedersenvss
}

func Polynomial(p []*big.Int,z *big.Int) *big.Int {
	result := new(big.Int)
	power, _ := new(big.Int).SetString("1", 10)
	for _, ele := range p {
		tmp := new(big.Int).Mul(ele, new(big.Int).Set(power))
		result.Add(result, tmp)
		power.Mul(power, z)
	}
	return result
}

func ModExp(base, exp, modulus *big.Int) *big.Int {
	exponent := new(big.Int)
	exponent, _ =  exponent.SetString(exp.String(), 10)
	result := big.NewInt(1)
	one := big.NewInt(1)
	for exponent.Cmp(one) > 0 {
		if exponent.Bit(0) == 1 {
			result.Mul(result, base)
			result.Mod(result, modulus)
		}
		base.Mul(base, base)
		base.Mod(base, modulus)
		exponent.Rsh(exponent, 1)
	}

	return result
}





// main
func main() {
	// client message sender, listen input from user, send messages to other clients
	messagesender := NewMessageSender(myIp, mySendPort)
	fmt.Println("[log]Your Node id =",findindex(allIPs,myIp))
	// check all nodes online
	go func() {
		if messagesender.onlinenum == len(allIPs)-1 {
			for _, value := range messagesender.Conns {
				value.Write([]byte("all nodes online\n"))
			}
			fmt.Println("[log]All Nodes Online!")
			messagesender.step1_generate()
		}
		return
	}() 
	// client message listener, listen messages from other clients
	messagelistener := NewMessageListener(myIp, myListenPort, messagesender)
	messagelistener.Start()
}

