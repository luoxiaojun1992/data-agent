db = db.getSiblingDB("data_agent");
var pcoll = db.rbac_permissions;
var rpcoll = db.rbac_role_permissions;
var now = new Date();
var admin = "rbac_role_admin";
var sysAdmin = "rbac_role_system_admin";

[{id:"rbac_perm_api_collection_view",key:"api:collection:view",name:"查看 API 集合"},
 {id:"rbac_perm_api_collection_edit",key:"api:collection:edit",name:"编辑 API 集合"},
 {id:"rbac_perm_api_collection_delete",key:"api:collection:delete",name:"删除 API 集合"},
 {id:"rbac_perm_api_collection_approve",key:"api:collection:approve",name:"审批 API 集合"},
 {id:"rbac_perm_admin_menu_api_collections",key:"admin:menu:api-collections",name:"API 管理入口"}]
.forEach(function(p) {
  pcoll.updateOne({_id: p.id}, {$set:{_id:p.id, key:p.key, name:p.name, module:"api", type:"builtin", created_at:now, updated_at:now}}, {upsert:true});
});

["api:collection:view","api:collection:edit","api:collection:delete","admin:menu:api-collections"].forEach(function(key) {
  var p = pcoll.findOne({key: key}); if (p) rpcoll.updateOne({role_id: admin, permission_id: p._id}, {$setOnInsert: {role_id: admin, permission_id: p._id, created_at: now}}, {upsert: true});
});
["api:collection:approve"].forEach(function(key) {
  var p = pcoll.findOne({key: key}); if (p) rpcoll.updateOne({role_id: sysAdmin, permission_id: p._id}, {$setOnInsert: {role_id: sysAdmin, permission_id: p._id, created_at: now}}, {upsert: true});
});

print("Admin: " + rpcoll.countDocuments({role_id: admin}) + " / SysAdmin: " + rpcoll.countDocuments({role_id: sysAdmin}));
