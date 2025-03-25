# Changelog

All notable changes to this project will be documented in this file.

## [v1.2.0] - 2025-03-25

### Added
- 批量删除功能：现在可以通过传入ID数组批量删除多条记录
- 使用 `DELETE /{path_prefix}/delete` 接口，请求体为 `{"ids": [1, 2, 3]}`
- 完善了错误处理，支持自定义错误消息和状态码
- 更新了README文档，添加了批量操作的示例

### Fixed
- 修复了表名在路径处理过程中可能丢失的问题
- 修复了PostgreSQL和MySQL语法差异导致的SQL错误
- 改进了查询参数处理逻辑
- 增强了没有查询结果时的处理，返回空对象而不是错误

## [v1.1.7] - 历史版本

*版本历史请参考git提交记录* 