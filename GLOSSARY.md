# Unicorn

可复用的 Go 服务端模板。本文件只收录模板自身语境里才成立的概念（目前是内置的认证与授权领域），通用编程概念不在此列。

## Language

**User（用户）**:
能被系统认证的主体，由邮箱唯一标识，可以没有任何 **Credential**。
_Avoid_: 账户、account、会员、member

**Credential（凭据）**:
证明 **User** 身份的秘密，目前只有密码一种形态。
_Avoid_: 密钥、secret、token

**Principal（主体）**:
请求处理期间代表已验证 **User** 的身份快照，携带其标识与 **Role** 集合。
_Avoid_: 当前用户、currentUser、identity、login

**Role（角色）**:
附加在 **User** 上的粗粒度授权标签，随 **AccessToken** 一起传递。
_Avoid_: 权限、permission、scope、group、acl

**AccessToken（访问令牌）**:
证明 **Principal** 的短效 JWT，携带其标识与 **Role** 集合，服务端无状态校验。
_Avoid_: token、会话令牌、session

**RefreshToken（刷新令牌）**:
用于换取新 **AccessToken** 的长期不透明凭据，服务端只保存其哈希，可轮换、可吊销。
_Avoid_: refresh jwt、refresh key、续期票据
