package utils

import (
	"sync"
	"time"
)

// 雪花算法配置
const (
	workerIDBits     = uint(5)  // 机器ID位数
	datacenterIDBits = uint(5)  // 数据中心ID位数
	sequenceBits     = uint(12) // 序列号位数

	maxWorkerID     = -1 ^ (-1 << workerIDBits)     // 最大机器ID 31
	maxDatacenterID = -1 ^ (-1 << datacenterIDBits) // 最大数据中心ID 31
	maxSequence     = -1 ^ (-1 << sequenceBits)     // 最大序列 4095

	timeShift       = workerIDBits + datacenterIDBits + sequenceBits // 时间戳左移
	workerShift     = datacenterIDBits + sequenceBits                // 机器ID左移
	datacenterShift = sequenceBits                                   // 数据中心ID左移
)

var (
	epoch         = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli() // 起始时间戳
	mu            sync.Mutex
	lastTimestamp int64 = -1
	sequence      int64 = 0
	workerID      int64 = 1
	datacenterID  int64 = 1
)

// NextID 获取下一个雪花ID
func NextID() int64 {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now().UnixMilli()
	if now < lastTimestamp {
		return 0
	}

	if now == lastTimestamp {
		sequence = (sequence + 1) & maxSequence
		if sequence == 0 {
			for now <= lastTimestamp {
				now = time.Now().UnixMilli()
			}
		}
	} else {
		sequence = 0
	}

	lastTimestamp = now

	// 生成ID
	id := (now-epoch)<<timeShift |
		(datacenterID << datacenterShift) |
		(workerID << workerShift) |
		sequence

	return id
}
