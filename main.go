package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	RequestCount := flag.Int("n", 4, "Request Count")
	hostPtr := flag.String("h", "", "Hostname or IP address")
	timeoutPtr := flag.Int("t", 1, "Timeout in seconds")
	helpPtr := flag.Bool("help", false, "Show help")
	bufferSizePtr := flag.Int("l", 32, "Send buffer size in bytes")
	flag.Parse()

	if *helpPtr {
		flag.Usage()
		return
	}

	if *hostPtr == "" {
		fmt.Println("Usage: mping -h <hostname or IP> -t <timeout> -l <buffer size>")
		return
	}

	host := *hostPtr

	ipAddr, err := net.ResolveIPAddr("ip", host)
	if err != nil {
		fmt.Printf("解析错误 %s: %s\n", host, err)
		return
	}

	dialer := net.Dialer{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				err := syscall.SetsockoptInt(syscall.Handle(int(fd)), syscall.SOL_SOCKET, syscall.SO_SNDBUF, *bufferSizePtr)
				if err != nil {
					fmt.Printf("设置发送缓冲区错误: %s\n", err)
				}
			})
		},
	}

	conn, err := dialer.Dial("ip4:icmp", ipAddr.String())
	if err != nil {
		fmt.Printf("创建连接错误: %s\n", err)
		return
	}
	defer conn.Close()

	pid := os.Getpid() & 0xffff
	seqNum := 1
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalChan
		fmt.Println("\n程序已终止.")
		os.Exit(0)
	}()

	fmt.Printf("Ping %s [%s] 具有%dbyte的数据 :\n", host, ipAddr.String(), *bufferSizePtr)

	REC := *RequestCount
	recvCount := 0
	Count := 0
	totalRTT := int64(0)
	minRTT := int64(1<<63 - 1)
	maxRTT := int64(0)

	Loss := 0

	for Count < *RequestCount {
		sendTime := time.Now()

		err = sendICMPPacket(conn, pid, seqNum)
		if err != nil {
			fmt.Printf("发包错误: %s\n", err)
			return
		}

		recvTime, err := receiveICMPPacket(conn, pid, seqNum, time.Second*time.Duration(*timeoutPtr))
		if err != nil {
			fmt.Printf("请求超时啊！\n")
			Count++
			Loss++
		} else {
			rtt := recvTime.Sub(sendTime).Milliseconds()
			fmt.Printf("收到回复 %s: size=%dbyte time=%dms\n", ipAddr.String(), *bufferSizePtr, rtt)
			Count++
			recvCount++
			totalRTT += rtt

			if rtt < minRTT {
				minRTT = rtt
			}
			if rtt > maxRTT {
				maxRTT = rtt
			}
		}

		time.Sleep(time.Second * 1)
		seqNum++
	}

	Cnt := (((REC - recvCount) * 100) / REC)
	fmt.Printf("\nPing %s结果:\n", host)
	fmt.Printf("    数据包: 发送 = %d, 收到 = %d, 丢失 = %d(%d%% 丢失率),\n", REC, recvCount, REC-recvCount, Cnt)
	if Loss < 4 {
		fmt.Printf("往返行程估计时间:\n")
		fmt.Printf("    最短 = %dms, 最长 = %dms, 平均 = %dms\n", minRTT, maxRTT, totalRTT/4)
	}
}

func sendICMPPacket(conn net.Conn, pid, seqNum int) error {
	payload := []byte("PingData")
	msg := make([]byte, 8+len(payload))
	msg[0] = 8 // ICMP Echo Request Type
	msg[1] = 0 // Code
	msg[2] = 0 // Checksum (set later)
	msg[3] = 0 // Checksum (set later)
	msg[4] = byte(pid >> 8)
	msg[5] = byte(pid)
	msg[6] = byte(seqNum >> 8)
	msg[7] = byte(seqNum)
	copy(msg[8:], payload)

	checksum := checksum(msg)
	msg[2] = byte(checksum >> 8)
	msg[3] = byte(checksum)

	_, err := conn.Write(msg)
	return err
}

// 在 receiveICMPPacket 函数中添加超时处理
func receiveICMPPacket(conn net.Conn, pid, seqNum int, timeout time.Duration) (time.Time, error) {
	recvBuffer := make([]byte, 1024)
	recvTime := time.Now()
	deadline := recvTime.Add(timeout)

	conn.SetReadDeadline(deadline) // 设置读取截止时间

	recvSize, err := conn.Read(recvBuffer)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return time.Time{}, fmt.Errorf("接收超时！")
		}
		return time.Time{}, err
	}

	conn.SetReadDeadline(time.Time{}) // 取消读取截止时间
	if recvSize < 20 {
		//包大小至少要能包含ICMP
		return time.Time{}, fmt.Errorf("ICMP packet too short")
	}

	msgType := recvBuffer[20]
	msgCode := recvBuffer[21]

	if msgType == 0 && msgCode == 0 {
		respPID := int(recvBuffer[24])<<8 | int(recvBuffer[25])
		respSeqNum := int(recvBuffer[26])<<8 | int(recvBuffer[27])

		if respPID == pid && respSeqNum == seqNum {
			return time.Now(), nil
		}
	}

	return time.Time{}, nil
}

func checksum(data []byte) uint16 {
	sum := 0
	for i := 0; i < len(data)-1; i += 2 {
		sum += int(data[i])<<8 | int(data[i+1])
	}

	if len(data)%2 == 1 {
		sum += int(data[len(data)-1]) << 8
	}

	sum = (sum >> 16) + (sum & 0xffff)
	sum = sum + (sum >> 16)
	return uint16(^sum)
}
