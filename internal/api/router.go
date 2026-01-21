// Package api 提供 OpenIM 的 HTTP API 服务实现。
//
// 该包负责初始化和管理 HTTP 服务器，处理客户端的 REST API 请求。
// 主要功能包括：
//   - HTTP 服务器的启动和配置
//   - 路由注册和请求分发
//   - 优雅关闭机制
//   - 服务健康检查
package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/openimsdk/open-im-server/v3/internal/api/jssdk"
	"github.com/openimsdk/open-im-server/v3/pkg/authverify"
	"github.com/openimsdk/open-im-server/v3/pkg/common/config"
	"github.com/openimsdk/open-im-server/v3/pkg/common/prommetrics"
	"github.com/openimsdk/open-im-server/v3/pkg/common/servererrs"
	"github.com/openimsdk/open-im-server/v3/pkg/rpcli"
	pbAuth "github.com/openimsdk/protocol/auth"
	"github.com/openimsdk/protocol/constant"
	"github.com/openimsdk/protocol/conversation"
	"github.com/openimsdk/protocol/group"
	"github.com/openimsdk/protocol/msg"
	"github.com/openimsdk/protocol/relation"
	"github.com/openimsdk/protocol/third"
	"github.com/openimsdk/protocol/user"
	"github.com/openimsdk/tools/apiresp"
	"github.com/openimsdk/tools/discovery"
	"github.com/openimsdk/tools/discovery/etcd"
	"github.com/openimsdk/tools/log"
	"github.com/openimsdk/tools/mw"
	"github.com/openimsdk/tools/mw/api"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// HTTP 响应压缩级别常量
const (
	NoCompression      = -1 // 不压缩
	DefaultCompression = 0  // 默认压缩级别
	BestCompression    = 1  // 最佳压缩率（压缩比最高，但速度较慢）
	BestSpeed          = 2  // 最快压缩速度（压缩比较低，但速度最快）
)

// prommetricsGin 是 Prometheus 监控中间件。
//
// 该中间件在每个请求完成后记录以下指标：
//   - HTTP 调用次数和状态码
//   - API 调用次数和错误码
//
// 返回值：
//   - gin.HandlerFunc: Gin 中间件函数
func prommetricsGin() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		path := c.FullPath()
		// 记录 HTTP 调用指标
		if c.Writer.Status() == http.StatusNotFound {
			prommetrics.HttpCall("<404>", c.Request.Method, c.Writer.Status())
		} else {
			prommetrics.HttpCall(path, c.Request.Method, c.Writer.Status())
		}
		// 记录 API 调用指标（包含业务错误码）
		if resp := apiresp.GetGinApiResponse(c); resp != nil {
			prommetrics.APICall(path, c.Request.Method, resp.ErrCode)
		}
	}
}

// newGinRouter 创建并配置 Gin 路由器。
//
// 该函数完成以下工作：
//  1. 建立与所有 RPC 服务的连接（auth、user、group、friend、conversation、third、msg）
//  2. 配置 Gin 引擎（发布模式、验证器、压缩等）
//  3. 注册全局中间件（日志、监控、错误恢复、CORS、Token 解析等）
//  4. 注册所有业务路由组：
//     - /user: 用户管理接口
//     - /friend: 好友关系接口
//     - /group: 群组管理接口
//     - /auth: 认证接口
//     - /third: 第三方服务接口
//     - /msg: 消息接口
//     - /conversation: 会话接口
//     - /statistics: 统计接口
//     - /jssdk: JS SDK 接口
//     - /prometheus_discovery: Prometheus 服务发现接口
//     - /config: 配置管理接口
//
// 参数：
//   - ctx: 上下文对象
//   - client: 服务发现注册客户端，用于获取 RPC 服务连接
//   - cfg: API 服务配置
//
// 返回值：
//   - *gin.Engine: 配置好的 Gin 路由器
//   - error: 初始化失败时返回错误
func newGinRouter(ctx context.Context, client discovery.SvcDiscoveryRegistry, cfg *Config) (*gin.Engine, error) {
	// 建立与认证服务的连接
	authConn, err := client.GetConn(ctx, cfg.Discovery.RpcService.Auth)
	if err != nil {
		return nil, err
	}
	// 建立与用户服务的连接
	userConn, err := client.GetConn(ctx, cfg.Discovery.RpcService.User)
	if err != nil {
		return nil, err
	}
	// 建立与群组服务的连接
	groupConn, err := client.GetConn(ctx, cfg.Discovery.RpcService.Group)
	if err != nil {
		return nil, err
	}
	// 建立与好友服务的连接
	friendConn, err := client.GetConn(ctx, cfg.Discovery.RpcService.Friend)
	if err != nil {
		return nil, err
	}
	// 建立与会话服务的连接
	conversationConn, err := client.GetConn(ctx, cfg.Discovery.RpcService.Conversation)
	if err != nil {
		return nil, err
	}
	// 建立与第三方服务的连接
	thirdConn, err := client.GetConn(ctx, cfg.Discovery.RpcService.Third)
	if err != nil {
		return nil, err
	}
	// 建立与消息服务的连接
	msgConn, err := client.GetConn(ctx, cfg.Discovery.RpcService.Msg)
	if err != nil {
		return nil, err
	}

	// 设置 Gin 为发布模式（减少日志输出）
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// 注册自定义验证器
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		_ = v.RegisterValidation("required_if", RequiredIf)
	}

	// 根据配置设置 HTTP 响应压缩级别
	switch cfg.API.Api.CompressionLevel {
	case NoCompression:
		// 不使用压缩
	case DefaultCompression:
		r.Use(gzip.Gzip(gzip.DefaultCompression))
	case BestCompression:
		r.Use(gzip.Gzip(gzip.BestCompression))
	case BestSpeed:
		r.Use(gzip.Gzip(gzip.BestSpeed))
	}

	// 单机模式下，直接设置管理员用户 ID 列表
	if config.Standalone() {
		r.Use(func(c *gin.Context) {
			c.Set(authverify.CtxAdminUserIDsKey, cfg.Share.IMAdminUser.UserIDs)
		})
	}

	// 注册全局中间件
	// - api.GinLogger(): 请求日志记录
	// - prommetricsGin(): Prometheus 监控指标
	// - gin.RecoveryWithWriter(): panic 恢复和错误处理
	// - mw.CorsHandler(): CORS 跨域处理
	// - mw.GinParseOperationID(): 解析操作 ID（用于日志追踪）
	// - GinParseToken(): Token 解析和验证
	// - setGinIsAdmin(): 设置管理员用户 ID 列表
	r.Use(api.GinLogger(), prommetricsGin(), gin.RecoveryWithWriter(gin.DefaultErrorWriter, mw.GinPanicErr), mw.CorsHandler(),
		mw.GinParseOperationID(), GinParseToken(rpcli.NewAuthClient(authConn)), setGinIsAdmin(cfg.Share.IMAdminUser.UserIDs))

	// 用户路由组 - 处理用户相关的所有接口
	u := NewUserApi(user.NewUserClient(userConn), client, cfg.Discovery.RpcService)
	{
		userRouterGroup := r.Group("/user")
		// 用户注册和信息管理
		userRouterGroup.POST("/user_register", u.UserRegister)                      // 用户注册
		userRouterGroup.POST("/update_user_info", u.UpdateUserInfo)                 // 更新用户信息
		userRouterGroup.POST("/update_user_info_ex", u.UpdateUserInfoEx)            // 更新用户扩展信息
		userRouterGroup.POST("/set_global_msg_recv_opt", u.SetGlobalRecvMessageOpt) // 设置全局消息接收选项
		userRouterGroup.POST("/get_users_info", u.GetUsersPublicInfo)               // 获取用户公开信息
		userRouterGroup.POST("/get_all_users_uid", u.GetAllUsersID)                 // 获取所有用户 ID
		userRouterGroup.POST("/account_check", u.AccountCheck)                      // 账号检查
		userRouterGroup.POST("/get_users", u.GetUsers)                              // 获取用户列表

		// 用户在线状态管理
		userRouterGroup.POST("/get_users_online_status", u.GetUsersOnlineStatus)            // 获取用户在线状态
		userRouterGroup.POST("/get_users_online_token_detail", u.GetUsersOnlineTokenDetail) // 获取用户在线 Token 详情
		userRouterGroup.POST("/subscribe_users_status", u.SubscriberStatus)                 // 订阅用户状态
		userRouterGroup.POST("/get_users_status", u.GetUserStatus)                          // 获取用户状态
		userRouterGroup.POST("/get_subscribe_users_status", u.GetSubscribeUsersStatus)      // 获取已订阅用户状态

		// 用户命令处理（管理员功能）
		userRouterGroup.POST("/process_user_command_add", u.ProcessUserCommandAdd)        // 添加用户命令
		userRouterGroup.POST("/process_user_command_delete", u.ProcessUserCommandDelete)  // 删除用户命令
		userRouterGroup.POST("/process_user_command_update", u.ProcessUserCommandUpdate)  // 更新用户命令
		userRouterGroup.POST("/process_user_command_get", u.ProcessUserCommandGet)        // 获取用户命令
		userRouterGroup.POST("/process_user_command_get_all", u.ProcessUserCommandGetAll) // 获取所有用户命令

		// 通知账号管理
		userRouterGroup.POST("/add_notification_account", u.AddNotificationAccount)           // 添加通知账号
		userRouterGroup.POST("/update_notification_account", u.UpdateNotificationAccountInfo) // 更新通知账号信息
		userRouterGroup.POST("/search_notification_account", u.SearchNotificationAccount)     // 搜索通知账号

		// 用户客户端配置管理
		userRouterGroup.POST("/get_user_client_config", u.GetUserClientConfig)   // 获取用户客户端配置
		userRouterGroup.POST("/set_user_client_config", u.SetUserClientConfig)   // 设置用户客户端配置
		userRouterGroup.POST("/del_user_client_config", u.DelUserClientConfig)   // 删除用户客户端配置
		userRouterGroup.POST("/page_user_client_config", u.PageUserClientConfig) // 分页获取用户客户端配置
	}

	// 好友路由组 - 处理好友关系相关的所有接口
	{
		f := NewFriendApi(relation.NewFriendClient(friendConn))
		friendRouterGroup := r.Group("/friend")
		// 好友管理
		friendRouterGroup.POST("/delete_friend", f.DeleteFriend)                            // 删除好友
		friendRouterGroup.POST("/get_friend_apply_list", f.GetFriendApplyList)              // 获取好友申请列表
		friendRouterGroup.POST("/get_designated_friend_apply", f.GetDesignatedFriendsApply) // 获取指定好友申请
		friendRouterGroup.POST("/get_self_friend_apply_list", f.GetSelfApplyList)           // 获取自己的好友申请列表
		friendRouterGroup.POST("/get_friend_list", f.GetFriendList)                         // 获取好友列表
		friendRouterGroup.POST("/get_designated_friends", f.GetDesignatedFriends)           // 获取指定好友信息
		friendRouterGroup.POST("/add_friend", f.ApplyToAddFriend)                           // 申请添加好友
		friendRouterGroup.POST("/add_friend_response", f.RespondFriendApply)                // 响应好友申请
		friendRouterGroup.POST("/set_friend_remark", f.SetFriendRemark)                     // 设置好友备注

		// 黑名单管理
		friendRouterGroup.POST("/add_black", f.AddBlack)                          // 添加黑名单
		friendRouterGroup.POST("/get_black_list", f.GetPaginationBlacks)          // 分页获取黑名单
		friendRouterGroup.POST("/get_specified_blacks", f.GetSpecifiedBlacks)     // 获取指定黑名单
		friendRouterGroup.POST("/remove_black", f.RemoveBlack)                    // 移除黑名单
		friendRouterGroup.POST("/get_incremental_blacks", f.GetIncrementalBlacks) // 增量获取黑名单

		// 好友高级功能
		friendRouterGroup.POST("/import_friend", f.ImportFriends)                               // 导入好友
		friendRouterGroup.POST("/is_friend", f.IsFriend)                                        // 判断是否为好友
		friendRouterGroup.POST("/get_friend_id", f.GetFriendIDs)                                // 获取好友 ID 列表
		friendRouterGroup.POST("/get_specified_friends_info", f.GetSpecifiedFriendsInfo)        // 获取指定好友详细信息
		friendRouterGroup.POST("/update_friends", f.UpdateFriends)                              // 批量更新好友
		friendRouterGroup.POST("/get_incremental_friends", f.GetIncrementalFriends)             // 增量获取好友
		friendRouterGroup.POST("/get_full_friend_user_ids", f.GetFullFriendUserIDs)             // 获取完整好友用户 ID 列表
		friendRouterGroup.POST("/get_self_unhandled_apply_count", f.GetSelfUnhandledApplyCount) // 获取未处理的好友申请数量
	}

	// 群组路由组 - 处理群组相关的所有接口
	g := NewGroupApi(group.NewGroupClient(groupConn))
	{
		groupRouterGroup := r.Group("/group")
		// 群组基本管理
		groupRouterGroup.POST("/create_group", g.CreateGroup)          // 创建群组
		groupRouterGroup.POST("/set_group_info", g.SetGroupInfo)       // 设置群组信息
		groupRouterGroup.POST("/set_group_info_ex", g.SetGroupInfoEx)  // 设置群组扩展信息
		groupRouterGroup.POST("/join_group", g.JoinGroup)              // 加入群组
		groupRouterGroup.POST("/quit_group", g.QuitGroup)              // 退出群组
		groupRouterGroup.POST("/transfer_group", g.TransferGroupOwner) // 转让群主
		groupRouterGroup.POST("/dismiss_group", g.DismissGroup)        // 解散群组

		// 群组申请管理
		groupRouterGroup.POST("/group_application_response", g.ApplicationGroupResponse)                     // 响应入群申请
		groupRouterGroup.POST("/get_recv_group_applicationList", g.GetRecvGroupApplicationList)              // 获取收到的入群申请列表
		groupRouterGroup.POST("/get_user_req_group_applicationList", g.GetUserReqGroupApplicationList)       // 获取用户发起的入群申请列表
		groupRouterGroup.POST("/get_group_users_req_application_list", g.GetGroupUsersReqApplicationList)    // 获取群组用户的入群申请列表
		groupRouterGroup.POST("/get_specified_user_group_request_info", g.GetSpecifiedUserGroupRequestInfo)  // 获取指定用户的入群申请信息
		groupRouterGroup.POST("/get_group_application_unhandled_count", g.GetGroupApplicationUnhandledCount) // 获取未处理的入群申请数量

		// 群组信息查询
		groupRouterGroup.POST("/get_groups_info", g.GetGroupsInfo)                // 获取群组信息
		groupRouterGroup.POST("/get_group_abstract_info", g.GetGroupAbstractInfo) // 获取群组摘要信息
		groupRouterGroup.POST("/get_groups", g.GetGroups)                         // 获取群组列表
		groupRouterGroup.POST("/get_joined_group_list", g.GetJoinedGroupList)     // 获取已加入的群组列表

		// 群成员管理
		groupRouterGroup.POST("/kick_group", g.KickGroupMember)                     // 踢出群成员
		groupRouterGroup.POST("/get_group_members_info", g.GetGroupMembersInfo)     // 获取群成员信息
		groupRouterGroup.POST("/get_group_member_list", g.GetGroupMemberList)       // 获取群成员列表
		groupRouterGroup.POST("/invite_user_to_group", g.InviteUserToGroup)         // 邀请用户入群
		groupRouterGroup.POST("/set_group_member_info", g.SetGroupMemberInfo)       // 设置群成员信息
		groupRouterGroup.POST("/get_group_member_user_id", g.GetGroupMemberUserIDs) // 获取群成员用户 ID 列表

		// 群组禁言管理
		groupRouterGroup.POST("/mute_group_member", g.MuteGroupMember)              // 禁言群成员
		groupRouterGroup.POST("/cancel_mute_group_member", g.CancelMuteGroupMember) // 取消禁言群成员
		groupRouterGroup.POST("/mute_group", g.MuteGroup)                           // 全员禁言
		groupRouterGroup.POST("/cancel_mute_group", g.CancelMuteGroup)              // 取消全员禁言

		// 群组增量同步
		groupRouterGroup.POST("/get_incremental_join_groups", g.GetIncrementalJoinGroup)                // 增量获取加入的群组
		groupRouterGroup.POST("/get_incremental_group_members", g.GetIncrementalGroupMember)            // 增量获取群成员
		groupRouterGroup.POST("/get_incremental_group_members_batch", g.GetIncrementalGroupMemberBatch) // 批量增量获取群成员
		groupRouterGroup.POST("/get_full_group_member_user_ids", g.GetFullGroupMemberUserIDs)           // 获取完整群成员用户 ID 列表
		groupRouterGroup.POST("/get_full_join_group_ids", g.GetFullJoinGroupIDs)                        // 获取完整加入的群组 ID 列表
	}

	// 认证路由组 - 处理用户认证相关的接口
	{
		a := NewAuthApi(pbAuth.NewAuthClient(authConn))
		authRouterGroup := r.Group("/auth")
		authRouterGroup.POST("/get_admin_token", a.GetAdminToken) // 获取管理员 Token
		authRouterGroup.POST("/get_user_token", a.GetUserToken)   // 获取用户 Token
		authRouterGroup.POST("/parse_token", a.ParseToken)        // 解析 Token
		authRouterGroup.POST("/force_logout", a.ForceLogout)      // 强制登出
	}

	// 第三方服务路由组 - 处理第三方服务集成相关的接口
	{
		t := NewThirdApi(third.NewThirdClient(thirdConn), cfg.API.Prometheus.GrafanaURL)
		thirdGroup := r.Group("/third")
		thirdGroup.GET("/prometheus", t.GetPrometheus)         // 获取 Prometheus 指标
		thirdGroup.POST("/fcm_update_token", t.FcmUpdateToken) // 更新 FCM Token
		thirdGroup.POST("/set_app_badge", t.SetAppBadge)       // 设置应用角标

		// 日志管理
		logs := thirdGroup.Group("/logs")
		logs.POST("/upload", t.UploadLogs) // 上传日志
		logs.POST("/delete", t.DeleteLogs) // 删除日志
		logs.POST("/search", t.SearchLogs) // 搜索日志

		// 对象存储管理
		objectGroup := r.Group("/object")
		objectGroup.POST("/part_limit", t.PartLimit)                              // 获取分片上传限制
		objectGroup.POST("/part_size", t.PartSize)                                // 获取分片大小
		objectGroup.POST("/initiate_multipart_upload", t.InitiateMultipartUpload) // 初始化分片上传
		objectGroup.POST("/auth_sign", t.AuthSign)                                // 生成授权签名
		objectGroup.POST("/complete_multipart_upload", t.CompleteMultipartUpload) // 完成分片上传
		objectGroup.POST("/access_url", t.AccessURL)                              // 获取访问 URL
		objectGroup.POST("/initiate_form_data", t.InitiateFormData)               // 初始化表单数据上传
		objectGroup.POST("/complete_form_data", t.CompleteFormData)               // 完成表单数据上传
		objectGroup.GET("/*name", t.ObjectRedirect)                               // 对象重定向
	}
	// 消息路由组 - 处理消息相关的所有接口
	m := NewMessageApi(msg.NewMsgClient(msgConn), rpcli.NewUserClient(userConn), cfg.Share.IMAdminUser.UserIDs)
	{
		msgGroup := r.Group("/msg")
		// 消息基本操作
		msgGroup.POST("/newest_seq", m.GetSeq)                                   // 获取最新序列号
		msgGroup.POST("/search_msg", m.SearchMsg)                                // 搜索消息
		msgGroup.POST("/send_msg", m.SendMessage)                                // 发送消息
		msgGroup.POST("/send_business_notification", m.SendBusinessNotification) // 发送业务通知
		msgGroup.POST("/pull_msg_by_seq", m.PullMsgBySeqs)                       // 根据序列号拉取消息
		msgGroup.POST("/revoke_msg", m.RevokeMsg)                                // 撤回消息
		msgGroup.POST("/batch_send_msg", m.BatchSendMsg)                         // 批量发送消息
		msgGroup.POST("/send_simple_msg", m.SendSimpleMessage)                   // 发送简单消息
		msgGroup.POST("/check_msg_is_send_success", m.CheckMsgIsSendSuccess)     // 检查消息是否发送成功
		msgGroup.POST("/get_server_time", m.GetServerTime)                       // 获取服务器时间

		// 消息已读管理
		msgGroup.POST("/mark_msgs_as_read", m.MarkMsgsAsRead)                                        // 标记消息为已读
		msgGroup.POST("/mark_conversation_as_read", m.MarkConversationAsRead)                        // 标记会话为已读
		msgGroup.POST("/get_conversations_has_read_and_max_seq", m.GetConversationsHasReadAndMaxSeq) // 获取会话已读和最大序列号
		msgGroup.POST("/set_conversation_has_read_seq", m.SetConversationHasReadSeq)                 // 设置会话已读序列号

		// 消息删除和清理
		msgGroup.POST("/clear_conversation_msg", m.ClearConversationsMsg)     // 清空会话消息
		msgGroup.POST("/user_clear_all_msg", m.UserClearAllMsg)               // 用户清空所有消息
		msgGroup.POST("/delete_msgs", m.DeleteMsgs)                           // 删除消息
		msgGroup.POST("/delete_msg_phsical_by_seq", m.DeleteMsgPhysicalBySeq) // 根据序列号物理删除消息
		msgGroup.POST("/delete_msg_physical", m.DeleteMsgPhysical)            // 物理删除消息
	}

	// 会话路由组 - 处理会话相关的所有接口
	{
		c := NewConversationApi(conversation.NewConversationClient(conversationConn))
		conversationGroup := r.Group("/conversation")
		conversationGroup.POST("/get_sorted_conversation_list", c.GetSortedConversationList)      // 获取排序后的会话列表
		conversationGroup.POST("/get_all_conversations", c.GetAllConversations)                   // 获取所有会话
		conversationGroup.POST("/get_conversation", c.GetConversation)                            // 获取单个会话
		conversationGroup.POST("/get_conversations", c.GetConversations)                          // 获取多个会话
		conversationGroup.POST("/set_conversations", c.SetConversations)                          // 设置会话属性
		conversationGroup.POST("/get_full_conversation_ids", c.GetFullOwnerConversationIDs)       // 获取完整会话 ID 列表
		conversationGroup.POST("/get_incremental_conversations", c.GetIncrementalConversation)    // 增量获取会话
		conversationGroup.POST("/get_owner_conversation", c.GetOwnerConversation)                 // 获取用户会话
		conversationGroup.POST("/get_not_notify_conversation_ids", c.GetNotNotifyConversationIDs) // 获取免打扰会话 ID 列表
		conversationGroup.POST("/get_pinned_conversation_ids", c.GetPinnedConversationIDs)        // 获取置顶会话 ID 列表
		//conversationGroup.POST("/get_conversation_offline_push_user_ids", c.GetConversationOfflinePushUserIDs) // 已注释：获取会话离线推送用户 ID
	}

	// 统计路由组 - 处理统计数据相关的接口
	{
		statisticsGroup := r.Group("/statistics")
		statisticsGroup.POST("/user/register", u.UserRegisterCount) // 用户注册统计
		statisticsGroup.POST("/user/active", m.GetActiveUser)       // 活跃用户统计
		statisticsGroup.POST("/group/create", g.GroupCreateCount)   // 群组创建统计
		statisticsGroup.POST("/group/active", m.GetActiveGroup)     // 活跃群组统计
	}

	// JS SDK 路由组 - 提供 JS SDK 专用接口
	{
		j := jssdk.NewJSSdkApi(rpcli.NewUserClient(userConn), rpcli.NewRelationClient(friendConn),
			rpcli.NewGroupClient(groupConn), rpcli.NewConversationClient(conversationConn), rpcli.NewMsgClient(msgConn))
		jssdk := r.Group("/jssdk")
		jssdk.POST("/get_conversations", j.GetConversations)              // 获取会话列表
		jssdk.POST("/get_active_conversations", j.GetActiveConversations) // 获取活跃会话列表
	}

	// Prometheus 服务发现路由组 - 提供服务发现接口供 Prometheus 使用
	{
		pd := NewPrometheusDiscoveryApi(cfg, client)
		proDiscoveryGroup := r.Group("/prometheus_discovery")
		proDiscoveryGroup.GET("/api", pd.Api)                      // API 服务发现
		proDiscoveryGroup.GET("/user", pd.User)                    // 用户服务发现
		proDiscoveryGroup.GET("/group", pd.Group)                  // 群组服务发现
		proDiscoveryGroup.GET("/msg", pd.Msg)                      // 消息服务发现
		proDiscoveryGroup.GET("/friend", pd.Friend)                // 好友服务发现
		proDiscoveryGroup.GET("/conversation", pd.Conversation)    // 会话服务发现
		proDiscoveryGroup.GET("/third", pd.Third)                  // 第三方服务发现
		proDiscoveryGroup.GET("/auth", pd.Auth)                    // 认证服务发现
		proDiscoveryGroup.GET("/push", pd.Push)                    // 推送服务发现
		proDiscoveryGroup.GET("/msg_gateway", pd.MessageGateway)   // 消息网关服务发现
		proDiscoveryGroup.GET("/msg_transfer", pd.MessageTransfer) // 消息传输服务发现
	}

	// 获取 ETCD 客户端（如果启用了 ETCD 服务发现）
	var etcdClient *clientv3.Client
	if cfg.Discovery.Enable == config.ETCD {
		etcdClient = client.(*etcd.SvcDiscoveryRegistryImpl).GetClient()
	}

	// 配置管理路由组 - 提供配置管理接口（需要管理员权限）
	cm := NewConfigManager(cfg.Share.IMAdminUser.UserIDs, &cfg.AllConfig, etcdClient, string(cfg.ConfigPath))
	{
		configGroup := r.Group("/config", cm.CheckAdmin)                          // 所有配置接口都需要管理员权限
		configGroup.POST("/get_config_list", cm.GetConfigList)                    // 获取配置列表
		configGroup.POST("/get_config", cm.GetConfig)                             // 获取配置
		configGroup.POST("/set_config", cm.SetConfig)                             // 设置配置
		configGroup.POST("/reset_config", cm.ResetConfig)                         // 重置配置
		configGroup.POST("/set_enable_config_manager", cm.SetEnableConfigManager) // 启用/禁用配置管理
		configGroup.POST("/get_enable_config_manager", cm.GetEnableConfigManager) // 获取配置管理状态
	}

	// 服务重启接口（需要管理员权限）
	{
		r.POST("/restart", cm.CheckAdmin, cm.Restart) // 重启服务
	}
	return r, nil
}

// GinParseToken 是 Token 解析和验证中间件。
//
// 该中间件在每个 POST 请求中执行以下操作：
//  1. 检查请求路径是否在白名单中，白名单中的接口不需要 Token
//  2. 从请求头中获取 Token
//  3. 调用认证服务验证 Token 的有效性
//  4. 将用户 ID 和平台 ID 存储到上下文中供后续处理使用
//
// 如果 Token 验证失败，请求将被中止并返回错误响应。
//
// 参数：
//   - authClient: 认证服务客户端，用于验证 Token
//
// 返回值：
//   - gin.HandlerFunc: Gin 中间件函数
func GinParseToken(authClient *rpcli.AuthClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodPost:
			// 检查是否在白名单中
			for _, wApi := range Whitelist {
				if strings.HasPrefix(c.Request.URL.Path, wApi) {
					c.Next()
					return
				}
			}

			// 从请求头获取 Token
			token := c.Request.Header.Get(constant.Token)
			if token == "" {
				log.ZWarn(c, "header get token error", servererrs.ErrArgs.WrapMsg("header must have token"))
				apiresp.GinError(c, servererrs.ErrArgs.WrapMsg("header must have token"))
				c.Abort()
				return
			}

			// 调用认证服务验证 Token
			resp, err := authClient.ParseToken(c, token)
			if err != nil {
				apiresp.GinError(c, err)
				c.Abort()
				return
			}

			// 将用户信息存储到上下文中
			c.Set(constant.OpUserPlatform, constant.PlatformIDToName(int(resp.PlatformID)))
			c.Set(constant.OpUserID, resp.UserID)
			c.Next()
		}
	}
}

// setGinIsAdmin 设置管理员用户 ID 列表到上下文中。
//
// 该中间件将管理员用户 ID 列表存储到每个请求的上下文中，
// 供后续的权限验证使用。
//
// 参数：
//   - imAdminUserID: 管理员用户 ID 列表
//
// 返回值：
//   - gin.HandlerFunc: Gin 中间件函数
func setGinIsAdmin(imAdminUserID []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(authverify.CtxAdminUserIDsKey, imAdminUserID)
	}
}

// Whitelist 定义了不需要 Token 验证的 API 白名单。
//
// 白名单中的接口可以在不提供 Token 的情况下访问，
// 通常包括登录、Token 解析等认证相关的接口。
var Whitelist = []string{
	"/auth/get_admin_token", // 获取管理员 Token
	"/auth/parse_token",     // 解析 Token
}
