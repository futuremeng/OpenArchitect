package unexported

type notAComponent struct{}

func use() notAComponent { return notAComponent{} }
