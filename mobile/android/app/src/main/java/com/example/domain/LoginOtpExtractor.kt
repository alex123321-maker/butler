package com.example.domain

import java.security.MessageDigest

/** In-memory SMS candidate only. No candidate is a grant or a queue record. */
class LoginOtpCandidate(val eventId: String, val sender: String, val code: String, val postedAt: Long) {
    override fun toString() = "LoginOtpCandidate([redacted])"
}

object LoginOtpExtractor {
    private val login = Regex("(?iu)(код.{0,24}(вход|авторизац)|вход.{0,24}код|login|sign.?in|verification code|security code)")
    private val excluded = Regex("(?isu)(плат[её]ж|оплат|перевод|списан|покупк|операци|(смен[аы]|изменени[ея]|сброс|восстановлени[ея]).{0,32}парол|парол.{0,32}(смен[аы]|изменени[ея]|сброс|восстановлени[ея])|payment|purchase|transfer|transaction|withdraw|password.{0,32}(reset|chang|updat)|(reset|chang|updat).{0,32}password)")
    private val digits = Regex("(?<![0-9])(?:[0-9]{4,8}|[0-9]{3}-[0-9]{3})(?![0-9])")

    fun extract(key: String, sender: String?, text: String, postedAt: Long, now: Long, platformRedacted: Boolean): LoginOtpCandidate? {
        if (platformRedacted || sender.isNullOrBlank() || sender.length > 200 || text.length > 8192 ||
            postedAt < now - 90_000 || postedAt > now + 5_000 || excluded.containsMatchIn(text) || !login.containsMatchIn(text)) return null
        val codes = digits.findAll(text).map { it.value.replace("-", "") }.toSet()
        if (codes.size != 1) return null
        val id = MessageDigest.getInstance("SHA-256").digest("$key|$postedAt".toByteArray()).joinToString("") { "%02x".format(it) }
        return LoginOtpCandidate(id, sender.trim(), codes.single(), postedAt)
    }
}
