pragma solidity ^0.8.4;

interface DarkForest {
    function SendEnergy(MsgSendEnergy calldata msg) external returns (MsgSendEnergyResponse calldata response);

    struct MsgSendEnergy {
        uint64 From;
        uint64 To;
        uint64 Amount;
    }

    struct MsgSendEnergyResponse {
        uint64 Code;
        string Message;
    }
}
