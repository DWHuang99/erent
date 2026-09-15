package apikey

type Checklist struct {
	checklist map[string]string
}

func NewChecklist() *Checklist {
	return &Checklist{checklist: make(map[string]string)}
}

func (c *Checklist) register(model, needOauth string) {
	c.checklist[model] = needOauth
}

func (c *Checklist) getNeedOauth(model string) string {
	return c.checklist[model]
}

var defaultchecklist = NewChecklist()

func Register(model, needOauth string) {
	defaultchecklist.register(model, needOauth)
}

func GetNeedOauth(model string) string {
	return defaultchecklist.getNeedOauth(model)
}
