package web

import (
	"net/http"

	"github.com/k8nstantin/OpenPraxis/internal/node"

	"github.com/gorilla/mux"
)

func apiMarkers(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		markers, err := n.Markers.ListForNode(n.PeerID(), status, 50)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, markers)
	}
}

func apiMarkerSeen(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if err := n.Markers.MarkSeen(id); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "seen"})
	}
}

func apiMarkerDone(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if err := n.Markers.MarkDone(id); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "done"})
	}
}
