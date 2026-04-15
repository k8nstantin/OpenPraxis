package web

import (
	"net/http"

	"openloom/internal/node"

	"github.com/gorilla/mux"
)

func apiMemories(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		prefix := r.URL.Query().Get("prefix")
		if prefix == "" {
			prefix = "/"
		}
		mems, err := n.Index.ListByPrefix(prefix, 100)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, mems)
	}
}

func apiSearch(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query   string `json:"query"`
			Scope   string `json:"scope"`
			Project string `json:"project"`
			Domain  string `json:"domain"`
			Limit   int    `json:"limit"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		if req.Limit <= 0 {
			req.Limit = 5
		}
		results, err := n.SearchMemories(r.Context(), req.Query, req.Limit, req.Scope, req.Project, req.Domain)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, results)
	}
}

func apiTree(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tree, err := n.Index.Tree()
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, tree)
	}
}

func apiMemoriesBySession(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mems, err := n.Index.ListByPrefix("/", 500)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}

		type memItem struct {
			ID        string `json:"id"`
			Marker    string `json:"marker"`
			L0        string `json:"l0"`
			Path      string `json:"path"`
			Type      string `json:"type"`
			CreatedAt string `json:"created_at"`
		}
		type sessionGroup struct {
			Session  string    `json:"session"`
			Memories []memItem `json:"memories"`
		}

		groups := make(map[string][]memItem)
		order := []string{}
		for _, m := range mems {
			src := m.SourceAgent
			if src == "" {
				src = "unknown"
			}
			marker := ""
			if len(m.ID) >= 12 {
				marker = m.ID[:12]
			}
			if _, ok := groups[src]; !ok {
				order = append(order, src)
			}
			groups[src] = append(groups[src], memItem{
				ID:        m.ID,
				Marker:    marker,
				L0:        m.L0,
				Path:      m.Path,
				Type:      m.Type,
				CreatedAt: m.CreatedAt,
			})
		}

		var result []sessionGroup
		for _, src := range order {
			result = append(result, sessionGroup{
				Session:  src,
				Memories: groups[src],
			})
		}
		writeJSON(w, result)
	}
}

func apiMemoriesByPeer(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mems, err := n.Index.ListByPrefix("/", 500)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}

		type memItem struct {
			ID        string `json:"id"`
			Marker    string `json:"marker"`
			L0        string `json:"l0"`
			Path      string `json:"path"`
			Type      string `json:"type"`
			CreatedAt string `json:"created_at"`
		}
		type sessionGroup struct {
			Session  string    `json:"session"`
			Count    int       `json:"count"`
			Memories []memItem `json:"memories"`
		}
		type peerGroup struct {
			PeerID   string         `json:"peer_id"`
			Count    int            `json:"count"`
			Sessions []sessionGroup `json:"sessions"`
		}

		// Group: peer -> session -> memories
		type peerData struct {
			sessionOrder []string
			sessions     map[string][]memItem
		}
		peers := make(map[string]*peerData)
		peerOrder := []string{}

		for _, m := range mems {
			peerID := m.SourceNode
			if peerID == "" {
				peerID = "unknown"
			}
			src := m.SourceAgent
			if src == "" {
				src = "unknown"
			}
			marker := ""
			if len(m.ID) >= 12 {
				marker = m.ID[:12]
			}

			pd, ok := peers[peerID]
			if !ok {
				pd = &peerData{sessions: make(map[string][]memItem)}
				peers[peerID] = pd
				peerOrder = append(peerOrder, peerID)
			}
			if _, ok := pd.sessions[src]; !ok {
				pd.sessionOrder = append(pd.sessionOrder, src)
			}
			pd.sessions[src] = append(pd.sessions[src], memItem{
				ID:        m.ID,
				Marker:    marker,
				L0:        m.L0,
				Path:      m.Path,
				Type:      m.Type,
				CreatedAt: m.CreatedAt,
			})
		}

		var result []peerGroup
		for _, pid := range peerOrder {
			pd := peers[pid]
			var sgs []sessionGroup
			totalCount := 0
			for _, src := range pd.sessionOrder {
				mlist := pd.sessions[src]
				// Already newest-first from DB query
				sgs = append(sgs, sessionGroup{
					Session:  src,
					Count:    len(mlist),
					Memories: mlist,
				})
				totalCount += len(mlist)
			}
			// Sort: find max timestamp per session, then sort descending
			type sortable struct {
				idx     int
				maxTime string
			}
			var ss []sortable
			for i, sg := range sgs {
				mt := ""
				for _, m := range sg.Memories {
					if m.CreatedAt > mt {
						mt = m.CreatedAt
					}
				}
				ss = append(ss, sortable{i, mt})
			}
			// Bubble sort descending by maxTime
			for i := 0; i < len(ss); i++ {
				for j := i + 1; j < len(ss); j++ {
					if ss[j].maxTime > ss[i].maxTime {
						ss[i], ss[j] = ss[j], ss[i]
					}
				}
			}
			sorted := make([]sessionGroup, len(sgs))
			for i, s := range ss {
				sorted[i] = sgs[s.idx]
			}
			sgs = sorted

			// Also sort memories within each session newest-first
			for k := range sgs {
				mems := sgs[k].Memories
				for i := 0; i < len(mems); i++ {
					for j := i + 1; j < len(mems); j++ {
						if mems[j].CreatedAt > mems[i].CreatedAt {
							mems[i], mems[j] = mems[j], mems[i]
						}
					}
				}
			}
			result = append(result, peerGroup{
				PeerID:   pid,
				Count:    totalCount,
				Sessions: sgs,
			})
		}
		writeJSON(w, result)
	}
}

func apiMemory(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		mem, err := n.Index.GetByID(id)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		if mem == nil {
			writeError(w, "not found", 404)
			return
		}
		writeJSON(w, mem)
	}
}

func apiMemoryDelete(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if err := n.Index.Delete(id); err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "deleted"})
	}
}
