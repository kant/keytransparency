// Copyright 2016 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package verifier

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/google/key-transparency/tree/sparse"
	"github.com/google/key-transparency/tree/sparse/sqlhist"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/net/context"
)

var (
	ctx          = context.Background()
	AllZeros     = strings.Repeat("0", 256/4)
	defaultIndex = []string{
		"8000000000000000000000000000000000000000000000000000000000000001",
		"C000000000000000000000000000000000000000000000000000000000000001",
		"4000000000000000000000000000000000000000000000000000000000000001",
	}
	hasher = sparse.CONIKSHasher
	// We insert all leaves and then commit once, so the epoch is 1.
	testEpoch = int64(1)
)

type Leaf struct {
	index  []byte
	value  []byte
	insert bool // Proof of absence.
}

type Env struct {
	db *sql.DB
	m  *sqlhist.Map
}

func NewEnv(leaves []Leaf) (*Env, error) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, fmt.Errorf("Failed creating in-memory sqlite3 db: %v", err)
	}
	m, err := sqlhist.New(db, "verify")
	if err != nil {
		return nil, fmt.Errorf("Failed to create SQL history: %v", err)
	}

	for _, leaf := range leaves {
		if leaf.insert {
			if err := m.QueueLeaf(ctx, leaf.index, leaf.value); err != nil {
				db.Close()
				return nil, fmt.Errorf("QueueLeaf(_, %v, %v)=%v", leaf.index, leaf.value, err)
			}
		}
	}
	_, err = m.Commit()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("Commit()=%v", err)
	}

	return &Env{db, m}, nil
}

func (e *Env) Close() {
	e.db.Close()
}

func TestVerifyProof(t *testing.T) {
	trees := [][]Leaf{
		{
			// Verify proof of absence in an empty tree.
			Leaf{dh(AllZeros), nil, false},
		},
		{
			Leaf{dh(defaultIndex[2]), []byte("0"), true},
			Leaf{dh(defaultIndex[0]), []byte("3"), true},
			Leaf{dh(AllZeros), nil, false},
		},
		{
			Leaf{dh(defaultIndex[0]), []byte("3"), true},
			Leaf{dh(defaultIndex[1]), []byte("4"), true},
			Leaf{dh(defaultIndex[2]), nil, false},
			Leaf{dh(AllZeros), nil, false},
		},
	}

	verifier := New(hasher)
	for _, leaves := range trees {
		// NewEnv will create a tree and fill it with all the leaves.
		env, err := NewEnv(leaves)
		if err != nil {
			t.Fatalf("NewEnv()=%v", err)
		}
		defer env.Close()

		root, err := env.m.ReadRootAt(ctx, testEpoch)
		if err != nil {
			t.Fatalf("ReadRootAt()=%v", err)
		}

		// VerifyProof of each leaf in the tree.
		for _, leaf := range leaves {
			nbrs, err := env.m.NeighborsAt(ctx, leaf.index, testEpoch)
			if err != nil {
				t.Fatalf("NeighborsAt(%v)=%v", leaf.index, err)
			}

			err = verifier.VerifyProof(nbrs, leaf.index, leaf.value, root)
			if err != nil {
				t.Fatalf("VerifyProof(_, %v, _, _)=%v", leaf.index, err)
			}
		}
	}
}

// Hex to Bytes
func dh(h string) []byte {
	result, err := hex.DecodeString(h)
	if err != nil {
		panic("DecodeString failed")
	}
	return result
}