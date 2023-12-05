deploy:
	# enter f88296beaf8692b6f7421c4f4a949b91d00bba904864637c7bb9bc58b7b0517f for the private key.
	forge create Game -r localhost:8545 --interactive

# use the contract address as an argument to this call that you see in the terminal after deploying.
#
# example: make join 0xbf3dC4185B43afecD015afa616C78fcDAC610603
make join:
	# enter f88296beaf8692b6f7421c4f4a949b91d00bba904864637c7bb9bc58b7b0517f for the private key.
	cast send $(filter-out $@,$(MAKECMDGOALS)) "joinGame()(bool)" --rpc-url localhost:8545 --interactive

make move:
	# enter f88296beaf8692b6f7421c4f4a949b91d00bba904864637c7bb9bc58b7b0517f for the private key.
	cast send $(filter-out $@,$(MAKECMDGOALS)) "movePlayer(string)(bool)" "up" --rpc-url localhost:8545 --interactive

make result:
	# f88296beaf8692b6f7421c4f4a949b91d00bba904864637c7bb9bc58b7b0517f
	cast send $(filter-out $@,$(MAKECMDGOALS)) "getJoinResult(string)(bool, string, uint32)" "0x42a9bf8b6c7536b849b8370c4fd93c9f8f46aeeb4cb2328b08fdd90297e86d92"  --rpc-url localhost:8545 --interactive

make location:
	# f88296beaf8692b6f7421c4f4a949b91d00bba904864637c7bb9bc58b7b0517f
	cast send $(filter-out $@,$(MAKECMDGOALS)) "Location(string)(int64, int64)" "tyler"   --rpc-url localhost:8545 --interactive
