package main

import (
	"encoding/gob"
	"os"
	"strings"
)

// --- Minimal B-tree with splitting ---

const btreeMinDegree = 16 

type btreeNode struct {
	leaf   bool
	keys   []string
	values []string
	child  []*btreeNode
}

type BTree struct {
	root *btreeNode
}

func NewBTree() *BTree {
	return &BTree{root: &btreeNode{leaf: true}}
}

func (t *BTree) Insert(key, value []byte) {
	skey := string(key)
	sval := string(value)
	root := t.root
	if len(root.keys) == 2*btreeMinDegree-1 {
		newRoot := &btreeNode{leaf: false, child: []*btreeNode{root}}
		splitChild(newRoot, 0)
		t.root = newRoot
		insertNonFull(newRoot, skey, sval)
	} else {
		insertNonFull(root, skey, sval)
	}
}

func insertNonFull(n *btreeNode, key, value string) {
	i := len(n.keys) - 1
	if n.leaf {
		n.keys = append(n.keys, "")
		n.values = append(n.values, "")
		for i >= 0 && key < n.keys[i] {
			n.keys[i+1] = n.keys[i]
			n.values[i+1] = n.values[i]
			i--
		}
		n.keys[i+1] = key
		n.values[i+1] = value
		return
	}
	for i >= 0 && key < n.keys[i] {
		i--
	}
	i++
	if len(n.child[i].keys) == 2*btreeMinDegree-1 {
		splitChild(n, i)
		if key > n.keys[i] {
			i++
		}
	}
	insertNonFull(n.child[i], key, value)
}

func splitChild(parent *btreeNode, i int) {
	t := btreeMinDegree
	full := parent.child[i]
	newNode := &btreeNode{leaf: full.leaf}
	newNode.keys = append(newNode.keys, full.keys[t:]...)
	newNode.values = append(newNode.values, full.values[t:]...)
	if !full.leaf {
		newNode.child = append(newNode.child, full.child[t:]...)
		full.child = full.child[:t]
	}
	midKey := full.keys[t-1]
	midVal := full.values[t-1]
	full.keys = full.keys[:t-1]
	full.values = full.values[:t-1]
	parent.keys = append(parent.keys, "")
	parent.values = append(parent.values, "")
	parent.child = append(parent.child, nil)
	copy(parent.keys[i+1:], parent.keys[i:])
	copy(parent.values[i+1:], parent.values[i:])
	copy(parent.child[i+2:], parent.child[i+1:])
	parent.keys[i] = midKey
	parent.values[i] = midVal
	parent.child[i+1] = newNode
}

func (t *BTree) Get(key []byte) ([]byte, bool) {
	skey := string(key)
	v, ok := btreeGet(t.root, skey)
	if !ok {
		return nil, false
	}
	return []byte(v), true
}

func btreeGet(n *btreeNode, key string) (string, bool) {
	if n == nil {
		return "", false
	}
	i := 0
	for i < len(n.keys) && key > n.keys[i] {
		i++
	}
	if i < len(n.keys) && key == n.keys[i] {
		return n.values[i], true
	}
	if n.leaf {
		return "", false
	}
	if i < len(n.child) {
		return btreeGet(n.child[i], key)
	}
	return "", false
}

func (t *BTree) Delete(key []byte) bool {
	// not done now
	return false
}

func (t *BTree) SaveToFile(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := gob.NewEncoder(f)
	return enc.Encode(t.root)
}

func (t *BTree) LoadFromFile(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	dec := gob.NewDecoder(f)
	root := &btreeNode{}
	if err := dec.Decode(root); err != nil {
		return err
	}
	t.root = root
	return nil
}

func (t *BTree) DebugPrint() {
	// fmt.Println("BTree dump:") 
	btreeDebugPrint(t.root, 0)
}

func btreeDebugPrint(n *btreeNode, level int) {
	if n == nil {
		return
	}
	// fmt.Printf("%sKeys: %v\n", indent(level), n.keys) 
	if !n.leaf {
		for _, c := range n.child {
			btreeDebugPrint(c, level+1)
		}
	}
}

func indent(n int) string {
	return strings.Repeat("  ", n)
}

