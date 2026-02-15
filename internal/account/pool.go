package account

import (
	"sort"
	"sync"

	"ds2api/internal/config"
)

type Pool struct {
	store *config.Store
	mu    sync.Mutex
	queue []string
	inUse map[string]bool
}

func NewPool(store *config.Store) *Pool {
	p := &Pool{store: store, inUse: map[string]bool{}}
	p.Reset()
	return p
}

func (p *Pool) Reset() {
	accounts := p.store.Accounts()
	sort.SliceStable(accounts, func(i, j int) bool {
		iHas := accounts[i].Token != ""
		jHas := accounts[j].Token != ""
		if iHas == jHas {
			return i < j
		}
		return iHas
	})
	ids := make([]string, 0, len(accounts))
	for _, a := range accounts {
		id := a.Identifier()
		if id != "" {
			ids = append(ids, id)
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.queue = ids
	p.inUse = map[string]bool{}
	config.Logger.Info("[init_account_queue] initialized", "total", len(ids))
}

func (p *Pool) Acquire(target string, exclude map[string]bool) (config.Account, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if target != "" {
		for i, id := range p.queue {
			if id != target {
				continue
			}
			acc, ok := p.store.FindAccount(id)
			if !ok {
				return config.Account{}, false
			}
			p.queue = append(p.queue[:i], p.queue[i+1:]...)
			p.inUse[id] = true
			return acc, true
		}
		return config.Account{}, false
	}

	for i := 0; i < len(p.queue); i++ {
		id := p.queue[i]
		if exclude[id] {
			continue
		}
		acc, ok := p.store.FindAccount(id)
		if !ok {
			continue
		}
		if acc.Token == "" {
			continue
		}
		p.queue = append(p.queue[:i], p.queue[i+1:]...)
		p.inUse[id] = true
		return acc, true
	}

	for i := 0; i < len(p.queue); i++ {
		id := p.queue[i]
		if exclude[id] {
			continue
		}
		acc, ok := p.store.FindAccount(id)
		if !ok {
			continue
		}
		p.queue = append(p.queue[:i], p.queue[i+1:]...)
		p.inUse[id] = true
		return acc, true
	}
	return config.Account{}, false
}

func (p *Pool) Release(accountID string) {
	if accountID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.inUse[accountID] {
		return
	}
	delete(p.inUse, accountID)
	p.queue = append(p.queue, accountID)
}

func (p *Pool) Status() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	available := append([]string{}, p.queue...)
	inUse := make([]string, 0, len(p.inUse))
	for id := range p.inUse {
		inUse = append(inUse, id)
	}
	return map[string]any{
		"available":          len(available),
		"in_use":             len(inUse),
		"total":              len(p.store.Accounts()),
		"available_accounts": available,
		"in_use_accounts":    inUse,
	}
}
