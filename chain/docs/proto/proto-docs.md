<!-- This file is auto-generated. Please do not modify it yourself. -->
# Protobuf Documentation
<a name="top"></a>

## Table of Contents

- [argus/adapter/v1/tx.proto](#argus/adapter/v1/tx.proto)
    - [MsgClaimQuestReward](#argus.adapter.v1.MsgClaimQuestReward)
    - [MsgClaimQuestRewardResponse](#argus.adapter.v1.MsgClaimQuestRewardResponse)
  
    - [Msg](#argus.adapter.v1.Msg)
  
- [argus/icamauth/v1beta1/query.proto](#argus/icamauth/v1beta1/query.proto)
    - [QueryInterchainAccountRequest](#argus.icamauth.v1beta1.QueryInterchainAccountRequest)
    - [QueryInterchainAccountResponse](#argus.icamauth.v1beta1.QueryInterchainAccountResponse)
  
    - [Query](#argus.icamauth.v1beta1.Query)
  
- [argus/icamauth/v1beta1/tx.proto](#argus/icamauth/v1beta1/tx.proto)
    - [MsgRegisterAccount](#argus.icamauth.v1beta1.MsgRegisterAccount)
    - [MsgRegisterAccountResponse](#argus.icamauth.v1beta1.MsgRegisterAccountResponse)
    - [MsgSubmitTx](#argus.icamauth.v1beta1.MsgSubmitTx)
    - [MsgSubmitTxResponse](#argus.icamauth.v1beta1.MsgSubmitTxResponse)
  
    - [Msg](#argus.icamauth.v1beta1.Msg)
  
- [Scalar Value Types](#scalar-value-types)



<a name="argus/adapter/v1/tx.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## argus/adapter/v1/tx.proto



<a name="argus.adapter.v1.MsgClaimQuestReward"></a>

### MsgClaimQuestReward
MsgClaimQuestReward is the Msg/ClaimQuestReward request type.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| `user_ID` | [string](#string) |  | user_ID is the game client user_ID. |
| `quest_ID` | [string](#string) |  | quest_ID is the ID of the quest that was completed. |






<a name="argus.adapter.v1.MsgClaimQuestRewardResponse"></a>

### MsgClaimQuestRewardResponse
MsgClaimQuestRewardResponse is the Msg/ClaimQuestReward response type.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| `reward_ID` | [string](#string) |  | reward_ID is the ID of the reward claimed. |





 <!-- end messages -->

 <!-- end enums -->

 <!-- end HasExtensions -->


<a name="argus.adapter.v1.Msg"></a>

### Msg


| Method Name | Request Type | Response Type | Description | HTTP Verb | Endpoint |
| ----------- | ------------ | ------------- | ------------| ------- | -------- |
| `ClaimQuestReward` | [MsgClaimQuestReward](#argus.adapter.v1.MsgClaimQuestReward) | [MsgClaimQuestRewardResponse](#argus.adapter.v1.MsgClaimQuestRewardResponse) | ClaimQuestReward claims a quest reward. | |

 <!-- end services -->



<a name="argus/icamauth/v1beta1/query.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## argus/icamauth/v1beta1/query.proto



<a name="argus.icamauth.v1beta1.QueryInterchainAccountRequest"></a>

### QueryInterchainAccountRequest
QueryInterchainAccountRequest is the request type for the Query/InterchainAccountAddress RPC


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| `owner` | [string](#string) |  |  |
| `connection_id` | [string](#string) |  |  |






<a name="argus.icamauth.v1beta1.QueryInterchainAccountResponse"></a>

### QueryInterchainAccountResponse
QueryInterchainAccountResponse the response type for the Query/InterchainAccountAddress RPC


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| `interchain_account_address` | [string](#string) |  |  |





 <!-- end messages -->

 <!-- end enums -->

 <!-- end HasExtensions -->


<a name="argus.icamauth.v1beta1.Query"></a>

### Query
Query defines the gRPC querier service.

| Method Name | Request Type | Response Type | Description | HTTP Verb | Endpoint |
| ----------- | ------------ | ------------- | ------------| ------- | -------- |
| `InterchainAccount` | [QueryInterchainAccountRequest](#argus.icamauth.v1beta1.QueryInterchainAccountRequest) | [QueryInterchainAccountResponse](#argus.icamauth.v1beta1.QueryInterchainAccountResponse) | QueryInterchainAccount returns the interchain account for given owner address on a given connection pair | GET|/argus/icamauth/v1beta1/interchain_account/owner/{owner}/connection/{connection_id}|

 <!-- end services -->



<a name="argus/icamauth/v1beta1/tx.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## argus/icamauth/v1beta1/tx.proto



<a name="argus.icamauth.v1beta1.MsgRegisterAccount"></a>

### MsgRegisterAccount
MsgRegisterAccount defines the payload for Msg/RegisterAccount


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| `owner` | [string](#string) |  |  |
| `connection_id` | [string](#string) |  |  |
| `version` | [string](#string) |  |  |






<a name="argus.icamauth.v1beta1.MsgRegisterAccountResponse"></a>

### MsgRegisterAccountResponse
MsgRegisterAccountResponse defines the response for Msg/RegisterAccount






<a name="argus.icamauth.v1beta1.MsgSubmitTx"></a>

### MsgSubmitTx
MsgSubmitTx defines the payload for Msg/SubmitTx


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| `owner` | [string](#string) |  |  |
| `connection_id` | [string](#string) |  |  |
| `msg` | [google.protobuf.Any](#google.protobuf.Any) |  |  |






<a name="argus.icamauth.v1beta1.MsgSubmitTxResponse"></a>

### MsgSubmitTxResponse
MsgSubmitTxResponse defines the response for Msg/SubmitTx





 <!-- end messages -->

 <!-- end enums -->

 <!-- end HasExtensions -->


<a name="argus.icamauth.v1beta1.Msg"></a>

### Msg
Msg defines the ica Msg service.

| Method Name | Request Type | Response Type | Description | HTTP Verb | Endpoint |
| ----------- | ------------ | ------------- | ------------| ------- | -------- |
| `RegisterAccount` | [MsgRegisterAccount](#argus.icamauth.v1beta1.MsgRegisterAccount) | [MsgRegisterAccountResponse](#argus.icamauth.v1beta1.MsgRegisterAccountResponse) | Register defines a rpc handler for MsgRegisterAccount | |
| `SubmitTx` | [MsgSubmitTx](#argus.icamauth.v1beta1.MsgSubmitTx) | [MsgSubmitTxResponse](#argus.icamauth.v1beta1.MsgSubmitTxResponse) | SubmitTx defines a rpc handler for MsgSubmitTx | |

 <!-- end services -->



## Scalar Value Types

| .proto Type | Notes | C++ | Java | Python | Go | C# | PHP | Ruby |
| ----------- | ----- | --- | ---- | ------ | -- | -- | --- | ---- |
| <a name="double" /> double |  | double | double | float | float64 | double | float | Float |
| <a name="float" /> float |  | float | float | float | float32 | float | float | Float |
| <a name="int32" /> int32 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint32 instead. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="int64" /> int64 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint64 instead. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="uint32" /> uint32 | Uses variable-length encoding. | uint32 | int | int/long | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="uint64" /> uint64 | Uses variable-length encoding. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum or Fixnum (as required) |
| <a name="sint32" /> sint32 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int32s. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sint64" /> sint64 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int64s. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="fixed32" /> fixed32 | Always four bytes. More efficient than uint32 if values are often greater than 2^28. | uint32 | int | int | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="fixed64" /> fixed64 | Always eight bytes. More efficient than uint64 if values are often greater than 2^56. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum |
| <a name="sfixed32" /> sfixed32 | Always four bytes. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sfixed64" /> sfixed64 | Always eight bytes. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="bool" /> bool |  | bool | boolean | boolean | bool | bool | boolean | TrueClass/FalseClass |
| <a name="string" /> string | A string must always contain UTF-8 encoded or 7-bit ASCII text. | string | String | str/unicode | string | string | string | String (UTF-8) |
| <a name="bytes" /> bytes | May contain any arbitrary sequence of bytes. | string | ByteString | str | []byte | ByteString | string | String (ASCII-8BIT) |

