package watcher

// {host:serviceName+servicePort}
type EventData struct {
	Host    string
	Service string
}

type EventHandler struct {
	OnAdd    func(data *EventData)
	OnUpdate func(oldData *EventData, newData *EventData)
	OnDelete func(data *EventData)
}
