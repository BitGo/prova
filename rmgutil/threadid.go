// Copyright (c) 2016 BitGo Inc.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rmgutil

import (
	"github.com/bitgo/rmgd/wire"
)

const RootThread = ThreadID(0)
const ProvisionThread = ThreadID(1)
const IssueThread = ThreadID(2)

type ThreadID uint8

func CopyThreadTips(threadTips map[ThreadID]*wire.OutPoint) map[ThreadID]*wire.OutPoint {
	threadTipsCopy := make(map[ThreadID]*wire.OutPoint)
	for threadId, outPoint := range threadTips {
		threadTipsCopy[threadId] = &wire.OutPoint{
			Index: outPoint.Index,
			Hash:  outPoint.Hash,
		}
	}
	return threadTipsCopy
}
