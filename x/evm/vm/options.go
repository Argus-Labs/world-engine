package vm

type AllowlistCheck func(addr string) bool

type ContractAllowlistOption struct {
	check AllowlistCheck
}

func NewContractAllowlistOption(check AllowlistCheck) ContractAllowlistOption {
	return ContractAllowlistOption{check: check}
}

func (c *ContractAllowlistOption) CanCreate(addr string) bool {
	return c.check(addr)
}
