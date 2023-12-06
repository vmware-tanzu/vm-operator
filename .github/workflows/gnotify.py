#!/usr/bin/env python3

import json
from httplib2 import Http
from os import environ
from types import SimpleNamespace


def get_issue_or_pr_html_link(obj):
    return '<a href="{0}">#{1} {2}</a>'.format(obj.html_url, obj.number, obj.title)


def get_issue_or_pr_text_link(obj):
    return "<{0}|#{1} {2}>".format(obj.html_url, obj.number, obj.title)


def get_user_text_link(user):
    return "<{0}|{1}>".format(user.html_url, user.login)


def get_user_html_link(user):
    return '<a href="{0}">{1}</a>'.format(user.html_url, user.login)


def get_labels(list):
    labels = []
    for l in list:
        labels.append('<font color="#{0}">{1}</font>'.format(l.color, l.name))
    if len(labels) == 0:
        labels.append("<i>None</i>")
    return ", ".join(labels)


def get_assignees(list):
    assignees = []
    for a in list:
        assignees.append(get_user_html_link(a))
    if len(assignees) == 0:
        assignees.append("<i>None</i>")
    return ", ".join(assignees)


def on_issue(event):
    msg_body = None
    reply_option = None

    if event.action == "opened":
        reply_option = "REPLY_MESSAGE_FALLBACK_TO_NEW_THREAD"
        msg_body = {
            "cardsV2": [
                {
                    "cardId": "issueCard-{0}".format(event.issue.number),
                    "card": {
                        "header": {
                            "title": "Issue opened by {0}".format(
                                event.issue.user.login
                            ),
                            "imageUrl": "https://github.githubassets.com/assets/GitHub-Mark-ea2971cee799.png",
                            "imageType": "CIRCLE",
                        },
                        "sections": [
                            {
                                "header": get_issue_or_pr_html_link(event.issue),
                                "widgets": [
                                    {
                                        "columns": {
                                            "columnItems": [
                                                {
                                                    "horizontalSizeStyle": "FILL_AVAILABLE_SPACE",
                                                    "horizontalAlignment": "START",
                                                    "verticalAlignment": "CENTER",
                                                    "widgets": [
                                                        {
                                                            "textParagraph": {
                                                                "text": "<b>Assignees</b><br>{0}".format(
                                                                    get_assignees(
                                                                        event.issue.assignees
                                                                    )
                                                                )
                                                            }
                                                        }
                                                    ],
                                                },
                                                {
                                                    "horizontalSizeStyle": "FILL_AVAILABLE_SPACE",
                                                    "horizontalAlignment": "START",
                                                    "verticalAlignment": "CENTER",
                                                    "widgets": [
                                                        {
                                                            "textParagraph": {
                                                                "text": "<b>Labels</b><br>{0}".format(
                                                                    get_labels(
                                                                        event.issue.labels
                                                                    )
                                                                )
                                                            }
                                                        }
                                                    ],
                                                },
                                            ]
                                        }
                                    }
                                ],
                            }
                        ],
                    },
                }
            ]
        }

    elif event.action == "labeled":
        msg_body = {
            "text": "👋 {0} added label {1} to issue {2}".format(
                get_user_text_link(event.sender),
                event.label.name,
                get_issue_or_pr_text_link(event.issue),
            )
        }

    elif event.action == "unlabeled":
        msg_body = {
            "text": "👋 {0} removed label {1} from issue {2}".format(
                get_user_text_link(event.sender),
                event.label.name,
                get_issue_or_pr_text_link(event.issue),
            )
        }

    elif event.action == "assigned":
        msg_body = {
            "text": "👋 {0} assigned {1} to issue {2}".format(
                get_user_text_link(event.sender),
                get_user_text_link(event.assignee),
                get_issue_or_pr_text_link(event.issue),
            )
        }

    elif event.action == "unassigned":
        msg_body = {
            "text": "👋 {0} unassigned {1} from issue {2}".format(
                get_user_text_link(event.sender),
                get_user_text_link(event.assignee),
                get_issue_or_pr_text_link(event.issue),
            )
        }

    else:
        msg_body = {
            "text": "👋 {0} {1} issue {2}".format(
                get_user_text_link(event.sender),
                event.action,
                get_issue_or_pr_text_link(event.issue),
            )
        }

    return event.issue.number, reply_option, msg_body


def on_issue_comment(event):
    return (
        event.issue.number,
        None,
        {
            "text": "👋 {0} left a <{1}|comment> on issue {2}".format(
                get_user_text_link(event.sender),
                event.comment.html_url,
                get_issue_or_pr_text_link(event.issue),
            )
        },
    )


def on_pull_request(event):
    msg_body = None
    reply_option = None

    if event.action == "opened":
        reply_option = "REPLY_MESSAGE_FALLBACK_TO_NEW_THREAD"
        msg_body = {
            "cardsV2": [
                {
                    "cardId": "pullRequestCard-{0}".format(event.pull_request.number),
                    "card": {
                        "header": {
                            "title": "Pull request opened by {0}".format(
                                event.issue.pull_request.login
                            ),
                            "imageUrl": "https://github.githubassets.com/assets/GitHub-Mark-ea2971cee799.png",
                            "imageType": "CIRCLE",
                        },
                        "sections": [
                            {
                                "header": get_issue_or_pr_html_link(event.pull_request),
                                "widgets": [
                                    {
                                        "columns": {
                                            "columnItems": [
                                                {
                                                    "horizontalSizeStyle": "FILL_AVAILABLE_SPACE",
                                                    "horizontalAlignment": "START",
                                                    "verticalAlignment": "CENTER",
                                                    "widgets": [
                                                        {
                                                            "textParagraph": {
                                                                "text": "<b>Assignees</b><br>{0}".format(
                                                                    get_assignees(
                                                                        event.pull_request.assignees
                                                                    )
                                                                )
                                                            }
                                                        }
                                                    ],
                                                },
                                                {
                                                    "horizontalSizeStyle": "FILL_AVAILABLE_SPACE",
                                                    "horizontalAlignment": "START",
                                                    "verticalAlignment": "CENTER",
                                                    "widgets": [
                                                        {
                                                            "textParagraph": {
                                                                "text": "<b>Labels</b><br>{0}".format(
                                                                    get_labels(
                                                                        event.pull_request.labels
                                                                    )
                                                                )
                                                            }
                                                        }
                                                    ],
                                                },
                                            ]
                                        }
                                    }
                                ],
                            }
                        ],
                    },
                }
            ]
        }

    elif event.action == "closed" and event.pull_request.merged:
        msg_body = {
            "text": "👋 {0} merged pull request {1}".format(
                get_user_text_link(event.sender),
                get_issue_or_pr_text_link(event.pull_request),
            )
        }

    elif event.action == "review_requested":
        msg_body = {
            "text": "👋 {0} requested {1} review pull request {2}".format(
                get_user_text_link(event.sender),
                get_user_text_link(event.requested_reviewer),
                get_issue_or_pr_text_link(event.pull_request),
            )
        }

    elif event.action == "labeled":
        msg_body = {
            "text": "👋 {0} added label {1} to pull request {2}".format(
                get_user_text_link(event.sender),
                event.label.name,
                get_issue_or_pr_text_link(event.pull_request),
            )
        }

    elif event.action == "unlabeled":
        msg_body = {
            "text": "👋 {0} removed label {1} from pull request {2}".format(
                get_user_text_link(event.sender),
                event.label.name,
                get_issue_or_pr_text_link(event.pull_request),
            )
        }

    elif event.action == "assigned":
        msg_body = {
            "text": "👋 {0} assigned {1} to pull request {2}".format(
                get_user_text_link(event.sender),
                get_user_text_link(event.assignee),
                get_issue_or_pr_text_link(event.pull_request),
            )
        }

    elif event.action == "unassigned":
        msg_body = {
            "text": "👋 {0} unassigned {1} from pull request {2}".format(
                get_user_text_link(event.sender),
                get_user_text_link(event.assignee),
                get_issue_or_pr_text_link(event.pull_request),
            )
        }

    else:
        msg_body = {
            "text": "👋 {0} {1} pull request {2}".format(
                get_user_text_link(event.sender),
                event.action,
                get_issue_or_pr_text_link(event.pull_request),
            )
        }

    return event.pull_request.number, reply_option, msg_body


def main():
    # Get the GitHub Action event payload.
    github = json.loads(
        environ.get("GITHUB_ACT_OBJCT"), object_hook=lambda d: SimpleNamespace(**d)
    )

    msg_body = None
    thread_key = None
    reply_option = None

    # The card data depends on the event that caused the action to fire.
    if github.event_name == "issues":
        thread_key, reply_option, msg_body = on_issue(github.event)
    elif github.event_name == "issue_comment":
        thread_key, reply_option, msg_body = on_issue_comment(github.event)
    elif github.event_name == "pull_request":
        thread_key, reply_option, msg_body = on_pull_request(github.event)

    # Please note this does not work as intended at the moment. Please see
    # https://issuetracker.google.com/issues/297308055?pli=1 for more info.
    # For now, if an issue or pull request are created with any labels or
    # assignees, those messages are likely to be sent before the message
    # announcing the issue or pull request. The latter is still sent, but it
    # will be threaded beneath the message related to an issue or assignee.
    if not reply_option:
        reply_option = "REPLY_MESSAGE_OR_FAIL"

    # Get the base URL with space, key, and token.
    url = environ.get("GOOGLE_SPACE_URL")

    # Update the URL with message reply options.
    url += "&messageReplyOption={0}".format(reply_option)

    msg_hdrs = {"Content-Type": "application/json; charset=UTF-8"}
    msg_body.update(
        {
            "thread": {"threadKey": "{0}".format(thread_key)},
        }
    )

    http_obj = Http()
    response = http_obj.request(
        uri=url,
        method="POST",
        headers=msg_hdrs,
        body=json.dumps(msg_body),
    )
    print(response)


if __name__ == "__main__":
    main()
