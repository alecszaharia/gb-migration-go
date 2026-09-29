
/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;
/*!40101 SET @OLD_CHARACTER_SET_RESULTS=@@CHARACTER_SET_RESULTS */;
/*!40101 SET @OLD_COLLATION_CONNECTION=@@COLLATION_CONNECTION */;
/*!50503 SET NAMES utf8mb4 */;
/*!40103 SET @OLD_TIME_ZONE=@@TIME_ZONE */;
/*!40103 SET TIME_ZONE='+00:00' */;
/*!40014 SET @OLD_UNIQUE_CHECKS=@@UNIQUE_CHECKS, UNIQUE_CHECKS=0 */;
/*!40014 SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0 */;
/*!40101 SET @OLD_SQL_MODE=@@SQL_MODE, SQL_MODE='NO_AUTO_VALUE_ON_ZERO' */;
/*!40111 SET @OLD_SQL_NOTES=@@SQL_NOTES, SQL_NOTES=0 */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `application` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `api_key` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `secret` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `client_id` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `scope` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `cms_application` (
  `id` int NOT NULL AUTO_INCREMENT,
  `author_id` int NOT NULL,
  `client_id` varchar(32) COLLATE utf8mb3_unicode_ci NOT NULL,
  `title` varchar(255) COLLATE utf8mb3_unicode_ci NOT NULL,
  `app_url` varchar(255) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  `default_installable` tinyint(1) NOT NULL,
  `category_id` int DEFAULT NULL,
  `type` varchar(255) COLLATE utf8mb3_unicode_ci NOT NULL,
  `status` varchar(20) COLLATE utf8mb3_unicode_ci DEFAULT 'new',
  `description` longtext COLLATE utf8mb3_unicode_ci,
  `tag_line` varchar(70) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `search_terms` varchar(300) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `demo_url` varchar(255) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `app_icon` varchar(255) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `notification_email` varchar(255) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `app_submission_contact_email` varchar(255) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `support_email` varchar(255) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `support_phone` varchar(255) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `privacy_policy_url` varchar(255) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `website_url` varchar(255) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `faq_url` varchar(255) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `review_instruction` varchar(2800) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `IDX_39BA6A15F675F31B` (`author_id`),
  KEY `IDX_39BA6A1519EB6921` (`client_id`),
  KEY `IDX_39BA6A1512469DE2` (`category_id`),
  CONSTRAINT `FK_39BA6A1512469DE2` FOREIGN KEY (`category_id`) REFERENCES `cms_application_category` (`id`),
  CONSTRAINT `FK_39BA6A1519EB6921` FOREIGN KEY (`client_id`) REFERENCES `oauth2_client` (`identifier`),
  CONSTRAINT `FK_39BA6A15F675F31B` FOREIGN KEY (`author_id`) REFERENCES `user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `cms_application_category` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(220) COLLATE utf8mb3_unicode_ci NOT NULL,
  `slug` varchar(220) COLLATE utf8mb3_unicode_ci NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `cms_application_install` (
  `id` int NOT NULL AUTO_INCREMENT,
  `project_id` int DEFAULT NULL,
  `application_id` int DEFAULT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  `author_id` int DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UNIQ_577F9A04166D1F9C3E030ACD` (`project_id`,`application_id`),
  KEY `IDX_577F9A04166D1F9C` (`project_id`),
  KEY `IDX_577F9A043E030ACD` (`application_id`),
  KEY `IDX_577F9A04F675F31B` (`author_id`),
  CONSTRAINT `FK_577F9A04166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_577F9A043E030ACD` FOREIGN KEY (`application_id`) REFERENCES `cms_application` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_577F9A04F675F31B` FOREIGN KEY (`author_id`) REFERENCES `user` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `collection_category` (
  `id` int NOT NULL AUTO_INCREMENT,
  `project_id` int NOT NULL,
  `created_at` datetime NOT NULL,
  `priority` int NOT NULL,
  `title` varchar(120) COLLATE utf8mb3_unicode_ci NOT NULL,
  PRIMARY KEY (`id`),
  KEY `IDX_F7CCD7F1166D1F9C` (`project_id`),
  KEY `IDX_F7CCD7F1166D1F9C62A6DC27` (`project_id`,`priority`),
  CONSTRAINT `FK_F7CCD7F1166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `collection_editor` (
  `id` int NOT NULL AUTO_INCREMENT,
  `project_id` int NOT NULL,
  `url` longtext COLLATE utf8mb3_unicode_ci NOT NULL,
  `created_at` datetime NOT NULL,
  `title` varchar(120) COLLATE utf8mb3_unicode_ci NOT NULL,
  `hidden` tinyint(1) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UNIQ_3ED4094C166D1F9C2B36786B` (`project_id`,`title`),
  KEY `IDX_3ED4094C166D1F9C` (`project_id`),
  CONSTRAINT `FK_3ED4094C166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `collection_item` (
  `id` int NOT NULL AUTO_INCREMENT,
  `type_id` int NOT NULL,
  `template` int DEFAULT NULL,
  `status` varchar(20) CHARACTER SET utf8mb3 COLLATE utf8mb3_unicode_ci NOT NULL,
  `created_at` datetime NOT NULL,
  `title` varchar(120) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `seo` json DEFAULT NULL,
  `slug` varchar(255) CHARACTER SET utf8mb3 COLLATE utf8mb3_unicode_ci NOT NULL,
  `project_id` int NOT NULL,
  `social` json DEFAULT NULL,
  `author_id` int NOT NULL,
  `updated_at` datetime DEFAULT NULL,
  `page_data_id` int DEFAULT NULL,
  `is_homepage` tinyint(1) NOT NULL DEFAULT '0',
  `code_injection` json DEFAULT NULL,
  `item_password` varchar(255) CHARACTER SET utf8mb3 COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `visibility` varchar(50) CHARACTER SET utf8mb3 COLLATE utf8mb3_unicode_ci NOT NULL DEFAULT 'public',
  `compiled_data_id` int DEFAULT NULL,
  `publish_date` datetime DEFAULT NULL,
  `dependencies` longtext CHARACTER SET utf8mb3 COLLATE utf8mb3_unicode_ci COMMENT '(DC2Type:simple_array)',
  `permalink` varchar(512) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `custom_permalink` tinyint(1) NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `UNIQ_556C09F0166D1F9C989D9B62` (`project_id`,`slug`),
  UNIQUE KEY `UNIQ_556C09F0CD954AA9` (`page_data_id`),
  UNIQUE KEY `UNIQ_556C09F094BCA57A` (`compiled_data_id`),
  UNIQUE KEY `UNIQ_556C09F0166D1F9CF286BC32` (`project_id`,`permalink`),
  KEY `IDX_556C09F0C54C8C93` (`type_id`),
  KEY `IDX_556C09F0166D1F9C` (`project_id`),
  KEY `IDX_556C09F0F675F31B` (`author_id`),
  KEY `IDX_556C09F0166D1F9CC54C8C93BF396750` (`project_id`,`type_id`,`id`),
  KEY `IDX_556C09F0166D1F9CBF396750` (`project_id`,`id`),
  KEY `IDX_556C09F0166D1F9CC54C8C932B36786B` (`project_id`,`type_id`,`title`),
  KEY `IDX_556C09F07B00651C78B553BA` (`status`,`publish_date`),
  CONSTRAINT `FK_556C09F0166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_556C09F094BCA57A` FOREIGN KEY (`compiled_data_id`) REFERENCES `compiled_data` (`id`) ON DELETE SET NULL,
  CONSTRAINT `FK_556C09F0C54C8C93` FOREIGN KEY (`type_id`) REFERENCES `collection_type` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_556C09F0CD954AA9` FOREIGN KEY (`page_data_id`) REFERENCES `page_data` (`id`) ON DELETE SET NULL,
  CONSTRAINT `FK_556C09F0F675F31B` FOREIGN KEY (`author_id`) REFERENCES `user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `collection_item_field` (
  `id` int NOT NULL AUTO_INCREMENT,
  `item_id` int NOT NULL,
  `type_id` int NOT NULL,
  `reference_id` int DEFAULT NULL,
  `values` json DEFAULT NULL,
  `project_id` int NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UNIQ_D2C8764C126F525EC54C8C93` (`item_id`,`type_id`),
  UNIQUE KEY `UNIQ_D2C8764C166D1F9CC54C8C93BF396750` (`project_id`,`type_id`,`id`),
  UNIQUE KEY `UNIQ_D2C8764C166D1F9C126F525EC54C8C93` (`project_id`,`item_id`,`type_id`),
  KEY `IDX_D2C8764C126F525E` (`item_id`),
  KEY `IDX_D2C8764C1645DEA9` (`reference_id`),
  KEY `IDX_D2C8764CC54C8C93` (`type_id`),
  KEY `IDX_D2C8764C166D1F9C` (`project_id`),
  KEY `IDX_D2C8764C166D1F9C126F525E` (`project_id`,`item_id`),
  KEY `IDX_D2C8764C1645DEA9126F525E` (`reference_id`,`item_id`),
  CONSTRAINT `FK_D2C8764C126F525E` FOREIGN KEY (`item_id`) REFERENCES `collection_item` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_D2C8764C1645DEA9` FOREIGN KEY (`reference_id`) REFERENCES `collection_item` (`id`) ON DELETE SET NULL,
  CONSTRAINT `FK_D2C8764C166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_D2C8764CC54C8C93` FOREIGN KEY (`type_id`) REFERENCES `collection_type_field` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `collection_item_field_multi_reference` (
  `collection_item_field_id` int NOT NULL,
  `collection_item_id` int NOT NULL,
  PRIMARY KEY (`collection_item_field_id`,`collection_item_id`),
  KEY `IDX_77087543F31B25B4` (`collection_item_field_id`),
  KEY `IDX_770875434643208F` (`collection_item_id`),
  KEY `IDX_mr_collection_item_field` (`collection_item_id`,`collection_item_field_id`),
  CONSTRAINT `FK_770875434643208F` FOREIGN KEY (`collection_item_id`) REFERENCES `collection_item` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_77087543F31B25B4` FOREIGN KEY (`collection_item_field_id`) REFERENCES `collection_item_field` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `collection_type` (
  `id` int NOT NULL AUTO_INCREMENT,
  `category_id` int DEFAULT NULL,
  `project_id` int NOT NULL,
  `created_at` datetime NOT NULL,
  `priority` int NOT NULL,
  `title` varchar(120) COLLATE utf8mb3_unicode_ci NOT NULL,
  `editor_id` int DEFAULT NULL,
  `settings` json DEFAULT NULL,
  `slug` varchar(255) COLLATE utf8mb3_unicode_ci NOT NULL,
  `has_preview` tinyint(1) NOT NULL DEFAULT '1',
  `public` tinyint(1) NOT NULL DEFAULT '1',
  `show_ui` tinyint(1) NOT NULL DEFAULT '1',
  `show_in_menu` tinyint(1) NOT NULL DEFAULT '1',
  `permalink_pattern` varchar(512) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UNIQ_C6A97BC7166D1F9C2B36786B` (`project_id`,`title`),
  UNIQUE KEY `UNIQ_C6A97BC7166D1F9C989D9B62` (`project_id`,`slug`),
  UNIQUE KEY `UNIQ_C6A97BC7166D1F9CBF396750` (`project_id`,`id`),
  KEY `IDX_C6A97BC712469DE2` (`category_id`),
  KEY `IDX_C6A97BC7166D1F9C` (`project_id`),
  KEY `IDX_C6A97BC7166D1F9C62A6DC27` (`project_id`,`priority`),
  KEY `IDX_C6A97BC76995AC4C` (`editor_id`),
  CONSTRAINT `FK_C6A97BC712469DE2` FOREIGN KEY (`category_id`) REFERENCES `collection_category` (`id`) ON DELETE SET NULL,
  CONSTRAINT `FK_C6A97BC7166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_C6A97BC76995AC4C` FOREIGN KEY (`editor_id`) REFERENCES `collection_editor` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `collection_type_field` (
  `id` int NOT NULL AUTO_INCREMENT,
  `collection_type_id` int NOT NULL,
  `type` varchar(30) COLLATE utf8mb3_unicode_ci NOT NULL,
  `required` tinyint(1) NOT NULL DEFAULT '0',
  `unique` tinyint(1) NOT NULL DEFAULT '0',
  `priority` int NOT NULL,
  `label` varchar(120) COLLATE utf8mb3_unicode_ci NOT NULL,
  `created_at` datetime NOT NULL,
  `settings` json DEFAULT NULL,
  `description` longtext COLLATE utf8mb3_unicode_ci,
  `placement` varchar(30) COLLATE utf8mb3_unicode_ci NOT NULL DEFAULT 'content',
  `reference_id` int DEFAULT NULL,
  `slug` varchar(255) COLLATE utf8mb3_unicode_ci NOT NULL,
  `hidden` tinyint(1) NOT NULL DEFAULT '0',
  `project_id` int NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UNIQ_F663EA6B9160FE42989D9B62` (`collection_type_id`,`slug`),
  UNIQUE KEY `UNIQ_F663EA6B166D1F9C9160FE42BF396750` (`project_id`,`collection_type_id`,`id`),
  KEY `IDX_F663EA6B9160FE42` (`collection_type_id`),
  KEY `IDX_F663EA6B1645DEA9` (`reference_id`),
  KEY `IDX_F663EA6B166D1F9C` (`project_id`),
  KEY `IDX_F663EA6B9160FE42166D1F9C62A6DC27` (`collection_type_id`,`project_id`,`priority`),
  KEY `IDX_F663EA6BBF396750166D1F9C62A6DC27` (`id`,`project_id`,`priority`),
  CONSTRAINT `FK_F663EA6B1645DEA9` FOREIGN KEY (`reference_id`) REFERENCES `collection_type` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_F663EA6B166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_F663EA6B9160FE42` FOREIGN KEY (`collection_type_id`) REFERENCES `collection_type` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `compiled_data` (
  `id` int NOT NULL AUTO_INCREMENT,
  `project_id` int NOT NULL,
  `ttl` int NOT NULL,
  `data` longtext COLLATE utf8mb4_unicode_ci NOT NULL,
  PRIMARY KEY (`id`),
  KEY `IDX_project_id` (`project_id`),
  KEY `IDX_ttl` (`ttl`),
  CONSTRAINT `FK_801985AC166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=COMPRESSED;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `customer` (
  `id` int NOT NULL AUTO_INCREMENT,
  `project_id` int NOT NULL,
  `email` varchar(100) COLLATE utf8mb3_unicode_ci NOT NULL,
  `first_name` varchar(50) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `last_name` varchar(50) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `phone` varchar(20) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `verified_email` tinyint(1) NOT NULL,
  `state` varchar(10) COLLATE utf8mb3_unicode_ci NOT NULL,
  `activation_token` varchar(100) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime DEFAULT NULL,
  `send_email_invite` tinyint(1) NOT NULL DEFAULT '0',
  `reset_password_token` varchar(100) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `reset_password_token_expire` datetime DEFAULT NULL,
  `password` varchar(255) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `is_email_invited` tinyint(1) NOT NULL DEFAULT '0',
  `user_name` varchar(40) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `page_data_id` int DEFAULT NULL,
  `code_injection` json DEFAULT NULL,
  `seo` json DEFAULT NULL,
  `social` json DEFAULT NULL,
  `compiled_data_id` int DEFAULT NULL,
  `dependencies` longtext COLLATE utf8mb3_unicode_ci COMMENT '(DC2Type:simple_array)',
  PRIMARY KEY (`id`),
  UNIQUE KEY `UNIQ_81398E09166D1F9CE7927C74` (`project_id`,`email`),
  UNIQUE KEY `UNIQ_81398E09166D1F9C24A232CF` (`project_id`,`user_name`),
  UNIQUE KEY `UNIQ_81398E09CD954AA9` (`page_data_id`),
  UNIQUE KEY `UNIQ_81398E0994BCA57A` (`compiled_data_id`),
  KEY `IDX_81398E09166D1F9C` (`project_id`),
  KEY `IDX_81398E09A9D1C132` (`first_name`),
  KEY `IDX_81398E09C808BA5A` (`last_name`),
  KEY `IDX_81398E09E7927C74` (`email`),
  KEY `IDX_81398E0924A232CF` (`user_name`),
  CONSTRAINT `FK_81398E09166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_81398E0994BCA57A` FOREIGN KEY (`compiled_data_id`) REFERENCES `compiled_data` (`id`) ON DELETE SET NULL,
  CONSTRAINT `FK_81398E09CD954AA9` FOREIGN KEY (`page_data_id`) REFERENCES `page_data` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `customer_group` (
  `id` int NOT NULL AUTO_INCREMENT,
  `project_id` int NOT NULL,
  `name` varchar(40) COLLATE utf8mb3_unicode_ci NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `IDX_A3F531FE166D1F9C` (`project_id`),
  KEY `IDX_A3F531FE166D1F9C5E237E06` (`project_id`,`name`),
  CONSTRAINT `FK_A3F531FE166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `customers_groups` (
  `customer_id` int NOT NULL,
  `customer_groups_id` int NOT NULL,
  PRIMARY KEY (`customer_id`,`customer_groups_id`),
  KEY `IDX_BB5A0D549395C3F3` (`customer_id`),
  KEY `IDX_BB5A0D54A4CAC5D5` (`customer_groups_id`),
  CONSTRAINT `FK_BB5A0D549395C3F3` FOREIGN KEY (`customer_id`) REFERENCES `customer` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_BB5A0D54A4CAC5D5` FOREIGN KEY (`customer_groups_id`) REFERENCES `customer_group` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `data` (
  `id` int NOT NULL AUTO_INCREMENT,
  `parent_id` int DEFAULT NULL,
  `node_id` int NOT NULL,
  `uid` varchar(255) CHARACTER SET utf8mb3 COLLATE utf8mb3_bin NOT NULL,
  `hash_id` varchar(255) CHARACTER SET utf8mb3 COLLATE utf8mb3_bin DEFAULT NULL,
  `lang_code` varchar(2) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'en',
  `status` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `title` varchar(255) CHARACTER SET utf8mb3 COLLATE utf8mb3_bin DEFAULT NULL,
  `slug` varchar(255) CHARACTER SET utf8mb3 COLLATE utf8mb3_bin DEFAULT NULL,
  `body` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci,
  `body_ref` varchar(255) CHARACTER SET utf8mb3 COLLATE utf8mb3_bin DEFAULT NULL,
  `author_id` int DEFAULT NULL,
  `version` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UNIQ_ADF3F363539B0606` (`uid`),
  KEY `IDX_ADF3F363727ACA70` (`parent_id`),
  KEY `IDX_ADF3F363460D9FD7` (`node_id`),
  KEY `node_author` (`node_id`,`author_id`),
  KEY `node_body_ref` (`node_id`,`body_ref`),
  KEY `node_title` (`node_id`,`title`),
  KEY `node_title_status` (`node_id`,`title`,`status`),
  CONSTRAINT `FK_ADF3F363460D9FD7` FOREIGN KEY (`node_id`) REFERENCES `node` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_ADF3F363727ACA70` FOREIGN KEY (`parent_id`) REFERENCES `data` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `data_user_role` (
  `id` int NOT NULL AUTO_INCREMENT,
  `data_id` int NOT NULL,
  `user_id` int DEFAULT NULL,
  `role_uid` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` int DEFAULT NULL,
  `token` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `timestamp` int DEFAULT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UNIQ_A247869537F5A13CA76ED395E7F98274` (`data_id`,`user_id`,`role_uid`),
  KEY `IDX_A247869537F5A13C` (`data_id`),
  KEY `IDX_A2478695A76ED395` (`user_id`),
  CONSTRAINT `FK_A247869537F5A13C` FOREIGN KEY (`data_id`) REFERENCES `data` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_A2478695A76ED395` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `external_page_data` (
  `id` int NOT NULL AUTO_INCREMENT,
  `project_id` int NOT NULL,
  `identifier` varchar(1024) COLLATE utf8mb4_unicode_ci NOT NULL,
  `data` varchar(1024) COLLATE utf8mb4_unicode_ci NOT NULL,
  `type` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  PRIMARY KEY (`id`),
  KEY `IDX_E6C40AD166D1F9C` (`project_id`),
  CONSTRAINT `FK_E6C40AD166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `global_block` (
  `id` int NOT NULL AUTO_INCREMENT,
  `page_data_id` int NOT NULL,
  `project_id` int NOT NULL,
  `uid` varchar(100) COLLATE utf8mb3_unicode_ci NOT NULL,
  `author_id` int NOT NULL,
  `title` varchar(120) COLLATE utf8mb3_unicode_ci NOT NULL,
  `status` varchar(20) COLLATE utf8mb3_unicode_ci NOT NULL,
  `meta` longtext COLLATE utf8mb3_unicode_ci NOT NULL,
  `position` longtext COLLATE utf8mb3_unicode_ci,
  `created_at` datetime NOT NULL,
  `updated_at` datetime DEFAULT NULL,
  `tags` longtext COLLATE utf8mb3_unicode_ci,
  `compiled_data_id` int DEFAULT NULL,
  `dependencies` json DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UNIQ_C71EED61CD954AA9` (`page_data_id`),
  UNIQUE KEY `UNIQ_C71EED6194BCA57A` (`compiled_data_id`),
  KEY `IDX_C71EED61166D1F9C` (`project_id`),
  KEY `IDX_C71EED61F675F31B` (`author_id`),
  KEY `IDX_C71EED61539B0606166D1F9C` (`uid`,`project_id`),
  CONSTRAINT `FK_C71EED61166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_C71EED6194BCA57A` FOREIGN KEY (`compiled_data_id`) REFERENCES `compiled_data` (`id`) ON DELETE SET NULL,
  CONSTRAINT `FK_C71EED61CD954AA9` FOREIGN KEY (`page_data_id`) REFERENCES `page_data` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_C71EED61F675F31B` FOREIGN KEY (`author_id`) REFERENCES `user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `lead` (
  `id` int NOT NULL AUTO_INCREMENT,
  `project_id` int NOT NULL,
  `data` longtext COLLATE utf8mb3_unicode_ci,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  KEY `IDX_289161CB166D1F9C` (`project_id`),
  CONSTRAINT `FK_289161CB166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `legacy_redirect_migration_tracking` (
  `id` int NOT NULL DEFAULT '1',
  `last_entity_id` int NOT NULL DEFAULT '0',
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `legacy_redirect_migration_tracking_chk_1` CHECK ((`id` = 1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `menu` (
  `id` int NOT NULL AUTO_INCREMENT,
  `project_id` int NOT NULL,
  `name` varchar(124) COLLATE utf8mb3_unicode_ci NOT NULL,
  `uid` varchar(64) COLLATE utf8mb3_unicode_ci NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UNIQ_7D053A93166053B4539B0606` (`project_id`,`uid`),
  KEY `IDX_7D053A93166053B4` (`project_id`),
  CONSTRAINT `FK_7D053A93166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `menu_item` (
  `id` int NOT NULL AUTO_INCREMENT,
  `menu_id` int NOT NULL,
  `item_id` int DEFAULT NULL,
  `parent_id` int DEFAULT NULL,
  `project_id` int NOT NULL,
  `type` int NOT NULL,
  `target` int NOT NULL,
  `url` varchar(2048) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `ecwid_product` varchar(64) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `ecwid_category` varchar(64) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `position` decimal(10,4) NOT NULL,
  `custom_label` varchar(512) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `enabled` tinyint(1) NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime DEFAULT NULL,
  `uid` varchar(64) COLLATE utf8mb3_unicode_ci NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UNIQ_D754D550166D1F9CCCD7E912539B0606` (`project_id`,`menu_id`,`uid`),
  KEY `IDX_D754D550166D1F9C` (`project_id`),
  KEY `IDX_D754D550CCD7E912` (`menu_id`),
  KEY `IDX_D754D550727ACA70` (`parent_id`),
  KEY `IDX_D754D550126F525E` (`item_id`),
  KEY `IDX_D754D550462CE4F5` (`position`),
  CONSTRAINT `FK_D754D550126F525E` FOREIGN KEY (`item_id`) REFERENCES `collection_item` (`id`) ON DELETE SET NULL,
  CONSTRAINT `FK_D754D550166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_D754D550727ACA70` FOREIGN KEY (`parent_id`) REFERENCES `menu_item` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_D754D550CCD7E912` FOREIGN KEY (`menu_id`) REFERENCES `menu` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `metafield` (
  `id` int NOT NULL AUTO_INCREMENT,
  `node_id` int NOT NULL,
  `name` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `type` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'int',
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UNIQ_95764600460D9FD75E237E06` (`node_id`,`name`),
  KEY `IDX_95764600460D9FD7` (`node_id`),
  CONSTRAINT `FK_95764600460D9FD7` FOREIGN KEY (`node_id`) REFERENCES `node` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `metafield__int` (
  `id` int NOT NULL AUTO_INCREMENT,
  `metafield_id` int DEFAULT NULL,
  `value` int NOT NULL,
  `entity_id` int NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UNIQ_E62ADE0C81257D5D7904F92F` (`entity_id`,`metafield_id`),
  KEY `IDX_E62ADE0C7904F92F` (`metafield_id`),
  CONSTRAINT `FK_E62ADE0C7904F92F` FOREIGN KEY (`metafield_id`) REFERENCES `metafield` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `metafield__text` (
  `id` int NOT NULL AUTO_INCREMENT,
  `metafield_id` int DEFAULT NULL,
  `value` longtext COLLATE utf8mb4_unicode_ci NOT NULL,
  `entity_id` int NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UNIQ_8EA913F281257D5D7904F92F` (`entity_id`,`metafield_id`),
  KEY `IDX_8EA913F27904F92F` (`metafield_id`),
  CONSTRAINT `FK_8EA913F27904F92F` FOREIGN KEY (`metafield_id`) REFERENCES `metafield` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `metafield__varchar` (
  `id` int NOT NULL AUTO_INCREMENT,
  `metafield_id` int DEFAULT NULL,
  `value` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `entity_id` int NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UNIQ_88BDD02A81257D5D7904F92F` (`entity_id`,`metafield_id`),
  KEY `IDX_88BDD02A7904F92F` (`metafield_id`),
  CONSTRAINT `FK_88BDD02A7904F92F` FOREIGN KEY (`metafield_id`) REFERENCES `metafield` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `migration_versions` (
  `version` varchar(255) COLLATE utf8mb3_unicode_ci NOT NULL,
  PRIMARY KEY (`version`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `migration_versions_graphql` (
  `version` varchar(191) COLLATE utf8mb3_unicode_ci NOT NULL,
  `executed_at` datetime DEFAULT NULL,
  `execution_time` int DEFAULT NULL,
  PRIMARY KEY (`version`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `node` (
  `id` int NOT NULL AUTO_INCREMENT,
  `parent_id` int DEFAULT NULL,
  `name` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `slug` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `entity_class` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `default_role_uid` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `is_file` tinyint(1) NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  KEY `IDX_857FE845727ACA70` (`parent_id`),
  CONSTRAINT `FK_857FE845727ACA70` FOREIGN KEY (`parent_id`) REFERENCES `node` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `oauth2_access_token` (
  `identifier` char(80) COLLATE utf8mb3_unicode_ci NOT NULL,
  `client` varchar(32) COLLATE utf8mb3_unicode_ci NOT NULL,
  `expiry` datetime NOT NULL COMMENT '(DC2Type:datetime_immutable)',
  `user_identifier` varchar(128) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `scopes` text COLLATE utf8mb3_unicode_ci COMMENT '(DC2Type:oauth2_scope)',
  `revoked` tinyint(1) NOT NULL,
  PRIMARY KEY (`identifier`),
  KEY `IDX_454D9673C7440455` (`client`),
  CONSTRAINT `FK_454D9673C7440455` FOREIGN KEY (`client`) REFERENCES `oauth2_client` (`identifier`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `oauth2_authorization_code` (
  `identifier` char(80) COLLATE utf8mb3_unicode_ci NOT NULL,
  `client` varchar(32) COLLATE utf8mb3_unicode_ci NOT NULL,
  `expiry` datetime NOT NULL COMMENT '(DC2Type:datetime_immutable)',
  `user_identifier` varchar(128) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `scopes` text COLLATE utf8mb3_unicode_ci COMMENT '(DC2Type:oauth2_scope)',
  `revoked` tinyint(1) NOT NULL,
  PRIMARY KEY (`identifier`),
  KEY `IDX_509FEF5FC7440455` (`client`),
  CONSTRAINT `FK_509FEF5FC7440455` FOREIGN KEY (`client`) REFERENCES `oauth2_client` (`identifier`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `oauth2_client` (
  `identifier` varchar(32) COLLATE utf8mb3_unicode_ci NOT NULL,
  `secret` varchar(128) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `redirect_uris` text COLLATE utf8mb3_unicode_ci COMMENT '(DC2Type:oauth2_redirect_uri)',
  `grants` text COLLATE utf8mb3_unicode_ci COMMENT '(DC2Type:oauth2_grant)',
  `scopes` text COLLATE utf8mb3_unicode_ci COMMENT '(DC2Type:oauth2_scope)',
  `active` tinyint(1) NOT NULL,
  `allow_plain_text_pkce` tinyint(1) NOT NULL DEFAULT '0',
  PRIMARY KEY (`identifier`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `oauth2_refresh_token` (
  `identifier` char(80) COLLATE utf8mb3_unicode_ci NOT NULL,
  `access_token` char(80) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `expiry` datetime NOT NULL COMMENT '(DC2Type:datetime_immutable)',
  `revoked` tinyint(1) NOT NULL,
  PRIMARY KEY (`identifier`),
  KEY `IDX_4DD90732B6A2DD68` (`access_token`),
  CONSTRAINT `FK_4DD90732B6A2DD68` FOREIGN KEY (`access_token`) REFERENCES `oauth2_access_token` (`identifier`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `page_data` (
  `id` int NOT NULL AUTO_INCREMENT,
  `project_id` int NOT NULL,
  `data` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  PRIMARY KEY (`id`),
  KEY `IDX_2A37EFB9166D1F9C` (`project_id`),
  CONSTRAINT `FK_2A37EFB9166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `permalink_recompute_tracking` (
  `id` int NOT NULL DEFAULT '1',
  `last_type_id` int NOT NULL DEFAULT '0',
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `permalink_recompute_tracking_chk_1` CHECK ((`id` = 1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `product` (
  `id` int NOT NULL AUTO_INCREMENT,
  `created_at` datetime NOT NULL,
  `seo` json DEFAULT NULL,
  `project_id` int NOT NULL,
  `sylius_id` int NOT NULL,
  `social` json DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UNIQ_D34A04AD166D1F9C2E6D65E9` (`project_id`,`sylius_id`),
  KEY `IDX_D34A04AD166D1F9C` (`project_id`),
  CONSTRAINT `FK_D34A04AD166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `project_access_token` (
  `id` int NOT NULL AUTO_INCREMENT,
  `access_token_id` char(80) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `project_id` int DEFAULT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  KEY `IDX_9C05390B2CCB2688` (`access_token_id`),
  KEY `IDX_9C05390B166D1F9C` (`project_id`),
  CONSTRAINT `FK_9C05390B166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_9C05390B2CCB2688` FOREIGN KEY (`access_token_id`) REFERENCES `oauth2_access_token` (`identifier`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `redirect` (
  `id` int NOT NULL AUTO_INCREMENT,
  `path` varchar(1024) COLLATE utf8mb3_unicode_ci NOT NULL,
  `target` varchar(1024) COLLATE utf8mb3_unicode_ci NOT NULL,
  `created_at` datetime NOT NULL,
  `project_id` int NOT NULL,
  `hash` varchar(32) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `redirect_status` int NOT NULL DEFAULT '301',
  `auto_generated` tinyint(1) NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `UNIQ_C30C9E2B166D1F9CD1B862B8` (`project_id`,`hash`),
  KEY `IDX_C30C9E2B166D1F9C` (`project_id`),
  CONSTRAINT `FK_C30C9E2B166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `revision` (
  `id` int NOT NULL AUTO_INCREMENT,
  `action` enum('create','update','remove') CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  `logged_at` datetime NOT NULL,
  `object_id` int DEFAULT NULL,
  `object_class` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  `version` int NOT NULL,
  `data` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_bin COMMENT '(DC2Type:array)',
  `username` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL,
  `ttl` int DEFAULT (unix_timestamp((now() + interval 30 day))),
  PRIMARY KEY (`id`),
  KEY `log_class_lookup_idx` (`object_class`),
  KEY `log_date_lookup_idx` (`logged_at`),
  KEY `log_user_lookup_idx` (`username`),
  KEY `log_version_lookup_idx` (`object_id`,`object_class`,`version`),
  KEY `ttl_idx` (`ttl`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin ROW_FORMAT=COMPRESSED;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `role` (
  `id` int NOT NULL AUTO_INCREMENT,
  `node_id` int NOT NULL,
  `name` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `uid` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `create_action` tinyint(1) NOT NULL,
  `read_action` tinyint(1) NOT NULL,
  `update_action` tinyint(1) NOT NULL,
  `delete_action` tinyint(1) NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  KEY `IDX_57698A6A460D9FD7` (`node_id`),
  CONSTRAINT `FK_57698A6A460D9FD7` FOREIGN KEY (`node_id`) REFERENCES `node` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `rules` (
  `id` int NOT NULL AUTO_INCREMENT,
  `global_block` int NOT NULL,
  `project_id` int NOT NULL,
  `collection_item` int DEFAULT NULL,
  `collection_type` int DEFAULT NULL,
  `customer` int DEFAULT NULL,
  `mode` varchar(7) COLLATE utf8mb3_unicode_ci NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime DEFAULT NULL,
  `type` varchar(10) COLLATE utf8mb3_unicode_ci NOT NULL,
  `collection_type_slug` varchar(255) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `external_type` varchar(32) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `external_id` varchar(32) COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `customer_group` int DEFAULT NULL,
  `collection_type_field` int DEFAULT NULL,
  `field_value_item` int DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `IDX_899A993CC71EED61` (`global_block`),
  KEY `IDX_899A993C166D1F9C` (`project_id`),
  KEY `IDX_899A993C556C09F0` (`collection_item`),
  KEY `IDX_899A993CC6A97BC7` (`collection_type`),
  KEY `IDX_899A993C81398E09` (`customer`),
  KEY `IDX_899A993CA3F531FE` (`customer_group`),
  KEY `IDX_899A993CF663EA6B` (`collection_type_field`),
  KEY `IDX_899A993CBB7C177F` (`field_value_item`),
  CONSTRAINT `FK_899A993C166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_899A993C556C09F0` FOREIGN KEY (`collection_item`) REFERENCES `collection_item` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_899A993C81398E09` FOREIGN KEY (`customer`) REFERENCES `customer` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_899A993CA3F531FE` FOREIGN KEY (`customer_group`) REFERENCES `customer_group` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_899A993CBB7C177F` FOREIGN KEY (`field_value_item`) REFERENCES `collection_item` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_899A993CC6A97BC7` FOREIGN KEY (`collection_type`) REFERENCES `collection_type` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_899A993CC71EED61` FOREIGN KEY (`global_block`) REFERENCES `global_block` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_899A993CF663EA6B` FOREIGN KEY (`collection_type_field`) REFERENCES `collection_type_field` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `script_tag` (
  `id` int NOT NULL AUTO_INCREMENT,
  `src` varchar(255) COLLATE utf8mb3_unicode_ci NOT NULL,
  `created_at` datetime NOT NULL,
  `project_id` int NOT NULL,
  PRIMARY KEY (`id`),
  KEY `IDX_3974EFC166D1F9C` (`project_id`),
  CONSTRAINT `FK_3974EFC166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `symbol` (
  `id` int NOT NULL AUTO_INCREMENT,
  `symbol_data_id` int DEFAULT NULL,
  `project_id` int NOT NULL,
  `uid` varchar(64) COLLATE utf8mb3_unicode_ci NOT NULL,
  `label` varchar(255) COLLATE utf8mb3_unicode_ci NOT NULL,
  `version` int NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime DEFAULT NULL,
  `class_name` varchar(255) COLLATE utf8mb3_unicode_ci NOT NULL,
  `component_target` varchar(255) COLLATE utf8mb3_unicode_ci NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UNIQ_ECC836F9166D1F9C539B0606` (`project_id`,`uid`),
  UNIQUE KEY `UNIQ_ECC836F978B5694F` (`symbol_data_id`),
  KEY `IDX_ECC836F9166D1F9C` (`project_id`),
  CONSTRAINT `FK_ECC836F9166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_ECC836F978B5694F` FOREIGN KEY (`symbol_data_id`) REFERENCES `symbol_data` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `symbol_data` (
  `id` int NOT NULL AUTO_INCREMENT,
  `project_id` int NOT NULL,
  `data` longtext COLLATE utf8mb3_unicode_ci NOT NULL,
  PRIMARY KEY (`id`),
  KEY `IDX_DC56D31166D1F9C` (`project_id`),
  CONSTRAINT `FK_DC56D31166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `template` (
  `id` int NOT NULL AUTO_INCREMENT,
  `type_id` int DEFAULT NULL,
  `project_id` int NOT NULL,
  `title` varchar(120) COLLATE utf8mb3_unicode_ci NOT NULL,
  `created_at` datetime NOT NULL,
  `data` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  PRIMARY KEY (`id`),
  KEY `IDX_97601F83C54C8C93` (`type_id`),
  KEY `IDX_97601F83166D1F9C` (`project_id`),
  CONSTRAINT `FK_97601F83166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_97601F83C54C8C93` FOREIGN KEY (`type_id`) REFERENCES `collection_type` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `user` (
  `id` int NOT NULL AUTO_INCREMENT,
  `application_id` int DEFAULT NULL,
  `node_id` int NOT NULL,
  `user_remote_id` int NOT NULL,
  `token` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `fingerprint` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `deleted_at` datetime DEFAULT NULL,
  `outdated` tinyint(1) NOT NULL,
  `products` longtext COLLATE utf8mb4_unicode_ci COMMENT '(DC2Type:json_array)',
  `locked` tinyint(1) NOT NULL DEFAULT '0',
  `approved` tinyint(1) NOT NULL DEFAULT '0',
  `status` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `cms_api_client_id` varchar(32) CHARACTER SET utf8mb3 COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `metadata` longtext COLLATE utf8mb4_unicode_ci,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UNIQ_8D93D6493E030ACD2ADE1170` (`application_id`,`user_remote_id`),
  KEY `IDX_8D93D6493E030ACD` (`application_id`),
  KEY `IDX_8D93D649460D9FD7` (`node_id`),
  KEY `IDX_8D93D649D46E778F` (`cms_api_client_id`),
  CONSTRAINT `FK_8D93D6493E030ACD` FOREIGN KEY (`application_id`) REFERENCES `application` (`id`),
  CONSTRAINT `FK_8D93D649460D9FD7` FOREIGN KEY (`node_id`) REFERENCES `node` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_8D93D649D46E778F` FOREIGN KEY (`cms_api_client_id`) REFERENCES `oauth2_client` (`identifier`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `webhook` (
  `id` int NOT NULL AUTO_INCREMENT,
  `project_id` int NOT NULL,
  `collection_type_id` int DEFAULT NULL,
  `url` varchar(255) COLLATE utf8mb3_unicode_ci NOT NULL,
  `object_name` varchar(255) COLLATE utf8mb3_unicode_ci NOT NULL,
  `hook_create` tinyint(1) NOT NULL DEFAULT '0',
  `hook_update` tinyint(1) NOT NULL DEFAULT '0',
  `hook_delete` tinyint(1) NOT NULL DEFAULT '0',
  `enabled` tinyint(1) NOT NULL DEFAULT '1',
  `created_at` datetime NOT NULL,
  `consecutive_failures` int unsigned NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  KEY `IDX_8A741756166D1F9C` (`project_id`),
  KEY `IDX_8A7417569160FE42` (`collection_type_id`),
  KEY `IDX_8A741756166D1F9C9160FE42` (`project_id`,`collection_type_id`),
  KEY `IDX_8A741756166D1F9C9160FE42F47645AE` (`project_id`,`collection_type_id`,`url`),
  CONSTRAINT `FK_8A741756166D1F9C` FOREIGN KEY (`project_id`) REFERENCES `data` (`id`) ON DELETE CASCADE,
  CONSTRAINT `FK_8A7417569160FE42` FOREIGN KEY (`collection_type_id`) REFERENCES `collection_type` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40103 SET TIME_ZONE=@OLD_TIME_ZONE */;

/*!40101 SET SQL_MODE=@OLD_SQL_MODE */;
/*!40014 SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS */;
/*!40014 SET UNIQUE_CHECKS=@OLD_UNIQUE_CHECKS */;
/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;
/*!40101 SET CHARACTER_SET_RESULTS=@OLD_CHARACTER_SET_RESULTS */;
/*!40101 SET COLLATION_CONNECTION=@OLD_COLLATION_CONNECTION */;
/*!40111 SET SQL_NOTES=@OLD_SQL_NOTES */;

