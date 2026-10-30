// UI E2E 预置 system_admin 账号（测试基建）。
//
// 与 cmd/server/main.go 的 ensureSystemAdmin 协同：data-agent 启动时会先
// HasSystemAdmin 检查是否已存在 role=system_admin 的用户，若存在则跳过随机
// 密码生成。因此本脚本先于 data-agent 执行，测试即可用固定密码稳定登录。
//
// 字段结构对齐 internal/infra/mongo/converter.go 的 userToDoc：
//   _id            -> 必须为 defaultSystemAdminUserID（rbac_seed.go:16 硬编码），
//                     否则 seedDefaultUserRole 绑定的 user_rbac_roles 指向空用户，
//                     system_admin 登录后无 RBAC 角色，无法调用 /users 等受保护接口
//   password_hash  -> bcrypt(DefaultCost=10)，明文 Admin@12345
//   role/status    -> system_admin / enabled
const admin = {
  _id: '6a64aba51214fe22b2cb917d',
  username: 'admin@admin.com',
  password_hash: '$2a$10$1IjnEISEucbDEZAobrDoIe2vi8xNj6smFM43UsaoGJJbJ6f5680G6',
  role: 'system_admin',
  status: 'enabled',
  password_changed: false,
  rbac_role_count: 0,
  created_at: new Date(),
  updated_at: new Date(),
};

const coll = db.getSiblingDB('data_agent').users;
const existing = coll.findOne({ username: admin.username });
if (existing) {
  print('seed-admin: ' + admin.username + ' already exists, skip');
} else {
  coll.insertOne(admin);
  print('seed-admin: seeded ' + admin.username + ' (password=Admin@12345)');
}
