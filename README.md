# go-timewheel

go时间轮，超时回调

### 安装

```bash
go get -u -v github.com/rrylee/go-timewheel
```

### Usage

```go
tw := NewTimeoutWheel()
ch := make(chan int, 3)

//注册一个20ms的超时回调
_, err := tw.Schedule(20*time.Millisecond, func(_ interface{}) { 
    ch <- 20 
}, nil)
if err != nil {
    panic(err)
}
tw.Stop()
```

### 参考

https://github.com/fagongzi/goetty
