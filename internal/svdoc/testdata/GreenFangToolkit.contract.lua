-- 依 GreenFangToolkitDB SavedVariables 格式（schemaVersion 2）手寫的測試樣本，所有資料皆為虛構。
-- 刻意包含：未知頂層欄位、未知 entry 欄位、未知 kind、缺 id 的壞 entry、
-- 不該上傳的 zone／online（GF Sync 會在上傳前移除）。
-- AddOn 線的正式樣本在 tests/fixtures/GreenFangToolkit.sample.lua。

GreenFangToolkitDB = {
	["schema"] = "gf.toolkit",
	["schemaVersion"] = 2,
	["addonVersion"] = "0.4.0",
	["futureTopLevel"] = {
		["whatever"] = 1,
	},
	["settings"] = {
		["minimapAngle"] = 200,
	},
	["sync"] = {
		["enabled"] = true,
		["noticeShown"] = true,
		["outbox"] = {
			{
				["id"] = "Player-9999-0000AAAA:1763370000:character.self",
				["kind"] = "character.self",
				["capturedAt"] = 1763370000,
				["futureField"] = "ignored by GF Sync, passed through",
				["game"] = {
					["world"] = "forever",
					["interface"] = 16001,
					["realm"] = "虛構伺服器",
					["locale"] = "zhTW",
				},
				["uploader"] = {
					["guid"] = "Player-9999-0000AAAA",
					["name"] = "虛構甲",
					["realm"] = "虛構伺服器",
					["faction"] = "Horde",
					["guild"] = "綠皮與獠牙冒險事務所",
					["guildRank"] = "會員",
				},
				["payload"] = {
					["level"] = 32,
					["class"] = "德魯伊",
					["classToken"] = "DRUID",
					["race"] = "牛頭人",
					["raceID"] = 6,
					["zone"] = "不該上傳",
					["professions"] = {
						{
							["name"] = "製皮",
							["skillLineID"] = 165,
							["rank"] = 151,
							["max"] = 225,
						}, -- [1]
					},
				},
			}, -- [1]
			{
				["id"] = "Player-9999-0000AAAA:1763370100:roster.snapshot",
				["kind"] = "roster.snapshot",
				["capturedAt"] = 1763370100,
				["game"] = {
					["world"] = "forever",
					["interface"] = 16001,
					["realm"] = "虛構伺服器",
					["locale"] = "zhTW",
				},
				["uploader"] = {
					["guid"] = "Player-9999-0000AAAA",
					["name"] = "虛構甲",
					["realm"] = "虛構伺服器",
					["faction"] = "Horde",
					["guild"] = "綠皮與獠牙冒險事務所",
					["guildRank"] = "會員",
				},
				["payload"] = {
					["total"] = 2,
					["members"] = {
						{
							["name"] = "虛構甲-虛構伺服器",
							["guid"] = "Player-9999-0000AAAA",
							["level"] = 32,
							["classToken"] = "DRUID",
							["guildRank"] = "會員",
							["professions"] = {
								"製皮", -- [1]
							},
							["professionSource"] = "guild",
							["zone"] = "不該上傳",
							["online"] = true,
						}, -- [1]
						{
							["name"] = "虛構乙-虛構伺服器",
							["guid"] = "Player-9999-0000BBBB",
							["level"] = 60,
							["classToken"] = "WARRIOR",
							["guildRank"] = "會長",
							["professions"] = {
							},
							["professionSource"] = "missing",
						}, -- [2]
					},
				},
			}, -- [2]
			{
				["id"] = "Player-9999-0000AAAA:1763370200:location.share",
				["kind"] = "location.share",
				["capturedAt"] = 1763370200,
				["payload"] = {
				},
			}, -- [3]
			{
				["kind"] = "character.self",
				["capturedAt"] = 1763370300,
			}, -- [4]
		},
	},
}
