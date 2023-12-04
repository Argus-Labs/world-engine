deploy:
	# enter f88296beaf8692b6f7421c4f4a949b91d00bba904864637c7bb9bc58b7b0517f for the private key.
	forge create Game -r localhost:8545 --interactive

# use the contract address as an argument to this call that you see in the terminal after deploying.
#
# example: make join 0xbf3dC4185B43afecD015afa616C78fcDAC610603
make join:
	# enter f88296beaf8692b6f7421c4f4a949b91d00bba904864637c7bb9bc58b7b0517f for the private key.
	cast send $(filter-out $@,$(MAKECMDGOALS)) "joinGame()(bool)" --rpc-url localhost:8545 --interactive

make result:
	cast send $(filter-out $@,$(MAKECMDGOALS)) "getJoinResult(string)(bool, string, uint32)" "0x180caba666fd7bfd26ba57c1344b12597adef3bb85f1cec796239abfa3252916"  --rpc-url localhost:8545 --interactive