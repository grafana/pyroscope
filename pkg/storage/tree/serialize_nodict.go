package tree

import "io"

func (t *Tree) SerializeNoDict(maxNodes int, w io.Writer) error {
	/*
		t.RLock()
		defer t.RUnlock()
			nodes := []*treeNode{t.root}
			minVal := t.minValue(maxNodes)
			j := 0

			for len(nodes) > 0 {
				j++
				tn := nodes[0]
				nodes = nodes[1:]

				label := t.loadLabel(tn.labelPosition)
				_, err := varint.Write(w, uint64(len(label)))
				if err != nil {
					return err
				}
				_, err = w.Write(label)
				if err != nil {
					return err
				}

				val := tn.Self
				_, err = varint.Write(w, val)
				if err != nil {
					return err
				}
				cnl := uint64(0)
				if tn.Total > minVal {
					cnl = uint64(len(tn.ChildrenNodes))
					nodes = append(tn.ChildrenNodes, nodes...)
				}
				_, err = varint.Write(w, cnl)
				if err != nil {
					return err
				}
			}*/
	return nil
}

func DeserializeNoDict(r io.Reader) (*Tree, error) {
	/*
		t := New()
		br := bufio.NewReader(r) // TODO if it's already a bytereader skip

		parents := []*parentNode{{t.root, nil}}
		j := 0

		for len(parents) > 0 {
			j++
			parent := parents[0]
			parents = parents[1:]

			nameLen, err := varint.Read(br)
			// if err == io.EOF {
			// 	return t, nil
			// }
			nameBuf := make([]byte, nameLen) // TODO: there are better ways to do this?
			_, err = io.ReadAtLeast(br, nameBuf, int(nameLen))
			if err != nil {
				return nil, err
			}
			tn := parent.node.insert(t, nameBuf)

			tn.Self, err = varint.Read(br)
			tn.Total = tn.Self
			if err != nil {
				return nil, err
			}

			pn := parent
			for pn != nil {
				pn.node.Total += tn.Self
				pn = pn.parent
			}

			childrenLen, err := varint.Read(br)
			if err != nil {
				return nil, err
			}

			for i := uint64(0); i < childrenLen; i++ {
				parents = append([]*parentNode{{tn, parent}}, parents...)
			}
		}

		t.root = t.root.ChildrenNodes[0]
	*/
	return nil, nil
}
